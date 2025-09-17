package guestcluster

import (
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"

	"github.com/harvester/harvester/pkg/util"
)

type VMController struct {
	vmClient     ctlkubevirtv1.VirtualMachineClient
	vmCache      ctlkubevirtv1.VirtualMachineCache
	vmController ctlkubevirtv1.VirtualMachineController

	guestClusterClient ctlharvesterv1.GuestClusterClient
	guestClusterCache  ctlharvesterv1.GuestClusterCache

	settingCache ctlharvesterv1.SettingCache
}

const (
	// to be repalced by public definitions
	guestClusterLabel = "guestcluster.harvesterhci.io/name"
	creatorLabel      = "harvesterhci.io/creator"
	creatorKey        = "docker-machine-driver-harvester"
)

func getGuestClusterVMInfo(vm *kubevirtv1.VirtualMachine) (string, bool) {
	gc := vm.Labels[guestClusterLabel]
	creator := vm.Labels[creatorLabel]
	if creator == creatorKey && gc != "" {
		return gc, true
	}
	return "", false
}

func (h *VMController) OnChange(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	gcName, ok := getGuestClusterVMInfo(vm)
	if !ok {
		return vm, nil
	}

	// find guest vm
	logrus.Infof("detect guest cluster: %s/%s", creatorKey, gcName)
	return vm, h.CreateOrUpdateGuestCluster(vm, gcName)
}

func (h *VMController) OnDelete(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil {
		return nil, nil
	}

	// to do: remove the potential recorded vm info
	gcName, ok := getGuestClusterVMInfo(vm)
	if !ok {
		return vm, nil
	}

	gc, err := h.guestClusterCache.Get(util.HarvesterSystemNamespaceName, gcName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		// vm is onDelete, but related guestCluster is not found, skip
		return vm, nil
	}

	// update
	_, ok = gc.Spec.Machines[vm.UID]
	if !ok {
		return vm, nil
	}

	// update guest cluster to remove this onDelete VM
	gcCopy := gc.DeepCopy()
	delete(gcCopy.Spec.Machines, vm.UID)
	if _, err := h.guestClusterClient.Update(gcCopy); err != nil {
		return nil, err
	}

	return vm, nil
}

func (h *VMController) CreateOrUpdateGuestCluster(vm *kubevirtv1.VirtualMachine, gcName string) error {
	gc, err := h.guestClusterCache.Get(util.HarvesterSystemNamespaceName, gcName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// create new
		return h.createGuestCluster(vm, gcName)
	}

	// already booked
	if _, ok := gc.Spec.Machines[vm.UID]; ok {
		return nil
	}

	// update guest cluster to add newly found VM
	gcCopy := gc.DeepCopy()
	obj := harvesterv1.ResourceInfo{
		Kind:         vm.Kind,
		APIVersion:   vm.APIVersion,
		Name:         vm.Name,
		Namespace:    vm.Namespace,
		GenerateName: vm.GenerateName,
		UID:          vm.UID,
	}
	gcCopy.Spec.Machines[vm.UID] = obj
	_, err = h.guestClusterClient.Update(gcCopy)
	return err
}

func (h *VMController) createGuestCluster(vm *kubevirtv1.VirtualMachine, gcName string) error {
	obj := harvesterv1.ResourceInfo{
		Kind:         vm.Kind,
		APIVersion:   vm.APIVersion,
		Name:         vm.Name,
		Namespace:    vm.Namespace,
		GenerateName: vm.GenerateName,
		UID:          vm.UID,
	}
	gc := &harvesterv1.GuestCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gcName,
			Namespace: util.HarvesterSystemNamespaceName,
		},
		Spec: harvesterv1.GuestClusterSpec{
			Machines: map[types.UID]harvesterv1.ResourceInfo{
				obj.UID: obj,
			},
		},
	}

	_, err := h.guestClusterClient.Create(gc)
	return err
}

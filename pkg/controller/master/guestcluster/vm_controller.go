package guestcluster

import (
	"github.com/sirupsen/logrus"

	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type VMController struct {
	vmClient     ctlkubevirtv1.VirtualMachineClient
	vmCache      ctlkubevirtv1.VirtualMachineCache
	vmController ctlkubevirtv1.VirtualMachineController

	settingCache ctlharvesterv1.SettingCache
}

const (
	guestClusterLabel = "guestcluster.harvesterhci.io/name"
	creatorLabel      = "harvesterhci.io/creator"
	creatorKey        = "docker-machine-driver-harvester"
)

// createPVCsFromAnnotation creates PVCs defined in the volumeClaimTemplates annotation if they don't exist.
func (h *VMController) OnChange(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	gc := vm.Labels[guestClusterLabel]
	creator := vm.Labels[creatorLabel]
	if creator == creatorKey && gc != "" {
		logrus.Infof("detector guest cluster: %s/%s", creator, gc)
	}

	return nil, nil
}

func (h *VMController) OnDelete(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	return nil, nil
}

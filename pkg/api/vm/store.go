package vm

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/sirupsen/logrus"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kv1 "kubevirt.io/client-go/api/v1"

	ctlcdiv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type vmStore struct {
	types.Store

	vms              ctlkubevirtv1.VirtualMachineClient
	vmCache          ctlkubevirtv1.VirtualMachineCache
	dataVolumes      ctlcdiv1beta1.DataVolumeClient
	dataVolumesCache ctlcdiv1beta1.DataVolumeCache
}

func (s *vmStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	removedDisks := request.Query["removedDisks"]
	vm, err := s.vmCache.Get(request.Namespace, request.Name)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to get vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	var savedDataVolumes []string
	var removedDataVolumes []string
	if vm.Spec.Template != nil {
		for _, vol := range vm.Spec.Template.Spec.Volumes {
			if vol.DataVolume == nil {
				continue
			}

			if slice.ContainsString(removedDisks, vol.Name) {
				removedDataVolumes = append(removedDataVolumes, vol.DataVolume.Name)
			} else {
				savedDataVolumes = append(savedDataVolumes, vol.DataVolume.Name)
			}
		}
	}

	if err = s.setRemovedDataVolumes(vm, removedDataVolumes); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to set removedDataVolumes to virtualMachine %s/%s, %v", request.Namespace, request.Name, err))
	}

	// Remove owner references on preserved DataVolumes to prevent being garbage-collected.
	// On the other hand, the owner references on removeDataVolumes will make them garbage-collected.
	if err = s.removeVMDataVolumeOwnerRef(vm.Namespace, vm.Name, savedDataVolumes); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove virtualMachine %s/%s from dataVolume's OwnerReferences, %v", request.Namespace, request.Name, err))
	}

	apiObj, err := s.Store.Delete(request, request.Schema, id)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	return apiObj, nil
}

func (s *vmStore) setRemovedDataVolumes(vm *kv1.VirtualMachine, removedDataVolumes []string) error {
	vmCopy := vm.DeepCopy()
	vmCopy.Annotations[util.RemovedDataVolumesAnnotationKey] = strings.Join(removedDataVolumes, ",")
	_, err := s.vms.Update(vmCopy)
	return err
}

func (s *vmStore) removeVMDataVolumeOwnerRef(vmNamespace, vmName string, savedDataVolumes []string) error {
	for _, volume := range savedDataVolumes {
		dv, err := s.dataVolumesCache.Get(vmNamespace, volume)
		if err != nil {
			if k8sapierrors.IsNotFound(err) {
				logrus.Infof("skip to remove owner reference, data volume %s not found", volume)
				continue
			}
			return err
		}

		var updatedOwnerRefs []metav1.OwnerReference
		for _, owner := range dv.OwnerReferences {
			if owner.Name == vmName && owner.Kind == kv1.VirtualMachineGroupVersionKind.Kind {
				continue
			}
			updatedOwnerRefs = append(updatedOwnerRefs, owner)
		}

		if len(updatedOwnerRefs) != len(dv.OwnerReferences) {
			copyDv := dv.DeepCopy()
			copyDv.OwnerReferences = updatedOwnerRefs
			if _, err = s.dataVolumes.Update(copyDv); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *vmStore) deleteDataVolumes(namespace string, names []string) error {
	for _, v := range names {
		if err := s.dataVolumes.Delete(namespace, v, &metav1.DeleteOptions{}); err != nil && !k8sapierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

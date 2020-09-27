package vm

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctlcdiv1beta1 "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
)

type vmStore struct {
	types.Store

	vmCache     ctlkubevirtv1alpha3.VirtualMachineCache
	dataVolumes ctlcdiv1beta1.DataVolumeClient
}

func (s *vmStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	removedDisks := request.Query["removedDisks"]
	vm, err := s.vmCache.Get(request.Namespace, request.Name)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to get vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	var savedDataVolumes []string
	var removedDataVolume []string
	if vm.Spec.Template != nil {
		for _, vol := range vm.Spec.Template.Spec.Volumes {
			if vol.DataVolume == nil {
				continue
			}

			if slice.ContainsString(removedDisks, vol.Name) {
				removedDataVolume = append(removedDataVolume, vol.DataVolume.Name)
			} else {
				savedDataVolumes = append(savedDataVolumes, vol.DataVolume.Name)
			}
		}
	}

	if err := s.removeVMDataVolumeOwnerRef(vm.Namespace, vm.Name, savedDataVolumes); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove virtualMachine %s/%s from dataVolume's OwnerReferences, %v", request.Namespace, request.Name, err))
	}

	apiObj, err := s.Store.Delete(request, request.Schema, id)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	if err := s.deleteDataVolumes(vm.Namespace, removedDataVolume); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove dataVolume, %v", err))
	}
	return apiObj, nil
}

func (s *vmStore) removeVMDataVolumeOwnerRef(vmNamespace, vmName string, savedDataVolumes []string) error {
	for _, dv := range savedDataVolumes {
		dv, err := s.dataVolumes.Get(vmNamespace, dv, metav1.GetOptions{})
		if err != nil {
			return err
		}

		var updatedOwnerRefs []metav1.OwnerReference
		for _, owner := range dv.OwnerReferences {
			if owner.Name == vmName && owner.Kind == "VirtualMachine" {
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

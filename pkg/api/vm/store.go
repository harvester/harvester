package vm

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/slice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

type vmStore struct {
	types.Store

	vms      ctlkubevirtv1.VirtualMachineClient
	vmCache  ctlkubevirtv1.VirtualMachineCache
	pvcs     v1.PersistentVolumeClaimClient
	pvcCache v1.PersistentVolumeClaimCache
}

func (s *vmStore) Delete(request *types.APIRequest, _ *types.APISchema, id string) (types.APIObject, error) {
	removedDisks := request.Query["removedDisks"]
	vm, err := s.vmCache.Get(request.Namespace, request.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return types.APIObject{}, apierror.NewAPIError(validation.NotFound, fmt.Sprintf("VirtualMachine %s/%s not found", request.Namespace, request.Name))
		}
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to get vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	var removedPVCs []string
	if vm.Spec.Template != nil {
		for _, vol := range vm.Spec.Template.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}

			if slice.ContainsString(removedDisks, vol.Name) {
				removedPVCs = append(removedPVCs, vol.PersistentVolumeClaim.ClaimName)
			}
		}
	}

	var filteredPVCs []string
	for _, v := range removedPVCs {
		ok, err := s.isPVCGoldenImage(vm.Namespace, v)
		if err != nil {
			return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to lookup pvc %s for vm %s/%s, %v", v, request.Namespace, request.Name, err))
		}
		if !ok {
			filteredPVCs = append(filteredPVCs, v)
		}
	}
	// Set removed PVCs in annotations. VMController is in charge of the cleanup.
	if err = s.setRemovedPVCs(vm, filteredPVCs); err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to set removedPersistentVolumeClaims to virtualMachine %s/%s, %v", request.Namespace, request.Name, err))
	}

	apiObj, err := s.Store.Delete(request, request.Schema, id)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove vm %s/%s, %v", request.Namespace, request.Name, err))
	}

	return apiObj, nil
}

func (s *vmStore) setRemovedPVCs(vm *kubevirtv1.VirtualMachine, removedPVCs []string) error {
	vmCopy := vm.DeepCopy()
	vmCopy.Annotations[util.RemovedPVCsAnnotationKey] = strings.Join(removedPVCs, ",")
	_, err := s.vms.Update(vmCopy)
	return err
}

// vm-import-controller allows golden images to be used natively as pvc with a vm
// during a delete operation we need to skip such pvc's as they are managed by the
// vmimage, and deletion of the vmimage will remove the pvc
func (s *vmStore) isPVCGoldenImage(namespace, name string) (bool, error) {
	pvcObj, err := s.pvcCache.Get(namespace, name)
	if err != nil {
		return false, fmt.Errorf("error looking up pvc %s/%s from cache: %w", name, namespace, err)
	}

	if pvcObj.Annotations == nil {
		return false, nil
	}
	if pvcObj.Annotations[util.AnnotationGoldenImage] == "true" {
		return true, nil
	}

	return false, nil
}

package vm

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/rancher/wrangler/pkg/slice"
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

func (s *vmStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	removedDisks := request.Query["removedDisks"]
	vm, err := s.vmCache.Get(request.Namespace, request.Name)
	if err != nil {
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

	// Set removed PVCs in annotations. VMController is in charge of the cleanup.
	if err = s.setRemovedPVCs(vm, removedPVCs); err != nil {
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

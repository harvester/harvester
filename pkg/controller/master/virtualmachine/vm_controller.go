package virtualmachine

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	kubevirtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	ctlcdiv1beta1 "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/harvester/pkg/indexeres"
	"github.com/rancher/harvester/pkg/ref"
)

type VMController struct {
	dataVolumeClient ctlcdiv1beta1.DataVolumeClient
	dataVolumeCache  ctlcdiv1beta1.DataVolumeCache
}

// SetOwnerOfDataVolumes records the target VirtualMachine as the owner of the DataVolumes in annotation.
func (h *VMController) SetOwnerOfDataVolumes(_ string, vm *kubevirtv1alpha3.VirtualMachine) (*kubevirtv1alpha3.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	dataVolumeNames := getDataVolumeNames(&vm.Spec.Template.Spec)

	vmReferenceKey := ref.Construct(vm.Namespace, vm.Name)
	// get all the volumes attached to this virtual machine
	attachedDataVolumes, err := h.dataVolumeCache.GetByIndex(indexeres.DataVolumeByVMIndex, vmReferenceKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get attached datavolumes by vm index: %w", err)
	}

	vmGK := kubevirtv1alpha3.VirtualMachineGroupVersionKind.GroupKind()
	for _, attachedDataVolume := range attachedDataVolumes {
		// check if it's still attached
		if dataVolumeNames.Has(attachedDataVolume.Name) {
			continue
		}
		// if this volume is no longer attached, update it's "owner-by" annotation
		owners, err := ref.GetSchemaOwnersFromAnnotation(attachedDataVolume)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema owners from annotation: %w", err)
		}

		if isOwned := owners.Delete(vmGK, vm); !isOwned {
			continue
		}

		toUpdate := attachedDataVolume.DeepCopy()
		if err := owners.Apply(toUpdate); err != nil {
			return nil, fmt.Errorf("failed to apply schema owners to annotation: %w", err)
		}

		if _, err = h.dataVolumeClient.Update(toUpdate); err != nil {
			return nil, fmt.Errorf("failed to clean schema owners for DataVolume(%s/%s): %w",
				attachedDataVolume.Namespace, attachedDataVolume.Name, err)
		}
	}

	var dataVolumeNamespace = vm.Namespace
	for _, dataVolumeName := range dataVolumeNames.List() {
		var dataVolume, err = h.dataVolumeCache.Get(dataVolumeNamespace, dataVolumeName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// NB(thxCode): ignores error, since this can't be fixed by an immediate requeue,
				// when the DataVolume is ready, the VirtualMachine resource will requeue.
				continue
			}
			return vm, fmt.Errorf("failed to get DataVolume(%s/%s): %w", dataVolumeNamespace, dataVolumeName, err)
		}

		err = setOwnerlessDataVolumeReference(h.dataVolumeClient, dataVolume, vm)
		if err != nil {
			return vm, fmt.Errorf("failed to grant VitrualMachine(%s/%s) as DataVolume(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, dataVolumeNamespace, dataVolumeName, err)
		}
	}

	return vm, nil
}

// UnsetOwnerOfDataVolumes erases the target VirtualMachine from the owner of the DataVolumes in annotation.
func (h *VMController) UnsetOwnerOfDataVolumes(_ string, vm *kubevirtv1alpha3.VirtualMachine) (*kubevirtv1alpha3.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp == nil || vm.Spec.Template == nil {
		return vm, nil
	}

	var dataVolumeNames = getDataVolumeNames(&vm.Spec.Template.Spec)

	var dataVolumeNamespace = vm.Namespace
	for _, dataVolumeName := range dataVolumeNames.List() {
		var dataVolume, err = h.dataVolumeCache.Get(dataVolumeNamespace, dataVolumeName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// NB(thxCode): ignores error, since this can't be fixed by an immediate requeue,
				// and also doesn't block the whole logic if the DataVolume has already been deleted.
				continue
			}
			return vm, fmt.Errorf("failed to get DataVolume(%s/%s): %w", dataVolumeNamespace, dataVolumeName, err)
		}

		err = unsetBoundedDataVolumeReference(h.dataVolumeClient, dataVolume, vm)
		if err != nil {
			return vm, fmt.Errorf("failed to revoke VitrualMachine(%s/%s) as DataVolume(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, dataVolumeNamespace, dataVolumeName, err)
		}
	}

	return vm, nil
}

// getDataVolumeNames returns a name set of the DataVolumes.
func getDataVolumeNames(vmiSpecPtr *kubevirtv1alpha3.VirtualMachineInstanceSpec) sets.String {
	var dataVolumeNames = sets.String{}

	// collects all DataVolumes
	for _, volume := range vmiSpecPtr.Volumes {
		if volume.DataVolume != nil && volume.DataVolume.Name != "" {
			dataVolumeNames.Insert(volume.DataVolume.Name)
		}
	}

	return dataVolumeNames
}

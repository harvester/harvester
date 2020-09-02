package virtualmachine

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	kubevirtapis "kubevirt.io/client-go/api/v1alpha3"

	cdictrl "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
)

type VMController struct {
	dataVolumeClient cdictrl.DataVolumeClient
	dataVolumeCache  cdictrl.DataVolumeCache
}

// SetOwnerOfDataVolumes records the target VirtualMachine as the owner of the DataVolumes in annotation.
func (h *VMController) SetOwnerOfDataVolumes(_ string, vm *kubevirtapis.VirtualMachine) (*kubevirtapis.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil || vm.Spec.Template == nil {
		return vm, nil
	}

	var dataVolumeNames = getDataVolumeNames(&vm.Spec.Template.Spec)

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
func (h *VMController) UnsetOwnerOfDataVolumes(_ string, vm *kubevirtapis.VirtualMachine) (*kubevirtapis.VirtualMachine, error) {
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
func getDataVolumeNames(vmiSpecPtr *kubevirtapis.VirtualMachineInstanceSpec) sets.String {
	var dataVolumeNames = sets.String{}

	// collects all DataVolumes
	for _, volume := range vmiSpecPtr.Volumes {
		if volume.DataVolume != nil && volume.DataVolume.Name != "" {
			dataVolumeNames.Insert(volume.DataVolume.Name)
		}
	}

	return dataVolumeNames
}

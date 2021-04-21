package virtualmachine

import (
	"fmt"
	"strings"

	"github.com/rancher/wrangler/pkg/slice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kv1 "kubevirt.io/client-go/api/v1"

	ctlcdiv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

type VMController struct {
	dataVolumeClient ctlcdiv1beta1.DataVolumeClient
	dataVolumeCache  ctlcdiv1beta1.DataVolumeCache
}

// SetOwnerOfDataVolumes records the target VirtualMachine as the owner of the DataVolumes in annotation.
func (h *VMController) SetOwnerOfDataVolumes(_ string, vm *kv1.VirtualMachine) (*kv1.VirtualMachine, error) {
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

	vmGVK := kv1.VirtualMachineGroupVersionKind
	vmAPIVersion, vmKind := vmGVK.ToAPIVersionAndKind()
	vmGK := vmGVK.GroupKind()

	for _, attachedDataVolume := range attachedDataVolumes {
		// check if it's still attached
		if dataVolumeNames.Has(attachedDataVolume.Name) {
			continue
		}

		toUpdate := attachedDataVolume.DeepCopy()

		// if this volume is no longer attached
		// remove volume's annotation
		owners, err := ref.GetSchemaOwnersFromAnnotation(attachedDataVolume)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema owners from annotation: %w", err)
		}

		isAttached := owners.Remove(vmGK, vm)
		if isAttached {
			if err := owners.Bind(toUpdate); err != nil {
				return nil, fmt.Errorf("failed to apply schema owners to annotation: %w", err)
			}
		}

		// remove volume's ownerReferences
		isOwned := false
		ownerReferences := make([]metav1.OwnerReference, 0, len(attachedDataVolume.OwnerReferences))
		for _, reference := range attachedDataVolume.OwnerReferences {
			if reference.APIVersion == vmAPIVersion && reference.Kind == vmKind && reference.Name == vm.Name {
				isOwned = true
				continue
			}
			ownerReferences = append(ownerReferences, reference)
		}
		if isOwned {
			if len(ownerReferences) == 0 {
				ownerReferences = nil
			}
			toUpdate.OwnerReferences = ownerReferences
		}

		// update volume
		if isAttached || isOwned {
			if _, err = h.dataVolumeClient.Update(toUpdate); err != nil {
				return nil, fmt.Errorf("failed to clean schema owners for DataVolume(%s/%s): %w",
					attachedDataVolume.Namespace, attachedDataVolume.Name, err)
			}
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
func (h *VMController) UnsetOwnerOfDataVolumes(_ string, vm *kv1.VirtualMachine) (*kv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp == nil || vm.Spec.Template == nil {
		return vm, nil
	}
	var (
		dataVolumeNamespace = vm.Namespace
		dataVolumeNames     = getDataVolumeNames(&vm.Spec.Template.Spec)
		removedDataVolumes  = getRemovedDataVolumes(vm)
	)
	for _, dataVolumeName := range dataVolumeNames.List() {
		if slice.ContainsString(removedDataVolumes, dataVolumeName) {
			// skip removedDataVolumes
			continue
		}
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

// getRemovedDataVolumes returns removed DataVolumes.
func getRemovedDataVolumes(vm *kv1.VirtualMachine) []string {
	return strings.Split(vm.Annotations[util.RemovedDataVolumesAnnotationKey], ",")
}

// getDataVolumeNames returns a name set of the DataVolumes.
func getDataVolumeNames(vmiSpecPtr *kv1.VirtualMachineInstanceSpec) sets.String {
	var dataVolumeNames = sets.String{}

	// collects all DataVolumes
	for _, volume := range vmiSpecPtr.Volumes {
		if volume.DataVolume != nil && volume.DataVolume.Name != "" {
			dataVolumeNames.Insert(volume.DataVolume.Name)
		}
	}

	return dataVolumeNames
}

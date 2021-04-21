package virtualmachine

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kubevirtapis "kubevirt.io/client-go/api/v1"

	cdictrl "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

type VMIController struct {
	virtualMachineCache kubevirtctrl.VirtualMachineCache
	dataVolumeClient    cdictrl.DataVolumeClient
	dataVolumeCache     cdictrl.DataVolumeCache
}

// UnsetOwnerOfDataVolumes erases the target VirtualMachine from the owner of the ownless DataVolumes in annotation.
//
// When modifying the VirtualMachine's spec to remove the previously defined DataVolumes and recreating the VirtualMachineInstance,
// those previously defined DataVolumes still hold an OwnerReference of the VirtualMachine,
// but they are no longer used by the newly created VirtualMachineInstance.
//
// Since the handler of VMController has recorded the relationship in the DataVolume's annotation,
// this handler will erase the target owner from the DataVolume's annotation to avoid logic leak.
//
//   ```diff
//     apiVersion: kubevirt.io/v1alpha3
//     kind: VirtualMachine
//     spec:
//   +   # the data-disk-2 DataVolume still be referred to this VirtualMachine until delete the whole VirtualMachine.
//   -   dataVolumeTemplates:
//   -   - metadata:
//   -     name: data-disk-2
//   -     spec:
//   -       pvc:
//   -         resource:
//   -           requests:
//   -             storage: 10Gi
//   -       source:
//   -         blank: {}
//       template:
//         spec:
//           domain:
//             devices:
//             - disk:
//               name: root-disk
//             - disk:
//               name: data-disk-1
//   -         - disk:
//   -           name: data-disk-2
//           volumes:
//           - dataVolume:
//             name: root-disk
//           - dataVolume:
//             name: data-disk-1
//   -       - dataVolume:
//   -         name: data-disk-2
//   ```
func (h *VMIController) UnsetOwnerOfDataVolumes(_ string, vmi *kubevirtapis.VirtualMachineInstance) (*kubevirtapis.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp == nil {
		return vmi, nil
	}

	// NB(thxCode): validate the VirtualMachineInstance's OwnerReference of a VirtualMachine,
	// and process only when it exists.
	var vmReferred = metav1.GetControllerOfNoCopy(vmi)
	if vmReferred == nil {
		// doesn't process ownerless VirtualMachineInstance
		return vmi, nil
	}
	var vmGVK = kubevirtapis.VirtualMachineGroupVersionKind
	if vmReferred.APIVersion != vmGVK.GroupVersion().String() ||
		vmReferred.Kind != vmGVK.Kind {
		// doesn't process the VirtualMachineInstance that didn't own by VirtualMachine
		return vmi, nil
	}

	var vm, err = h.virtualMachineCache.Get(vmi.Namespace, vmReferred.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// NB(thxCode): in this case, the related VirtualMachine is deleted,
			// which triggers the cascading deletion of the VirtualMachineInstance.
			// however, the deletion of the VirtualMachineInstance is placed in the background,
			// the VirtualMachineInstance is still deleting, but the corresponding VirtualMachine has been deleted.
			// so we can ignores this error.
			return vmi, nil
		}
		return vmi, fmt.Errorf("failed to get VirtualMachine referred by VirtualMachineInstance(%s/%s): %w", vmi.Namespace, vmi.Name, err)
	}
	if vm.DeletionTimestamp != nil {
		// reconciling executed by the VirtualMachine controller and don't process in here.
		return vmi, nil
	}

	var dataVolumeNames = sets.String{}
	if vmiDesired := vm.Spec.Template; vmiDesired != nil { // just a defend, the validating webhook of virt-api will make this never happen
		dataVolumeNames = getDataVolumeNames(&vmiDesired.Spec)
	}
	var dataVolumeNameObserved = getDataVolumeNames(&vmi.Spec)

	// unsets ownerless DataVolumes
	var dataVolumeNamespace = vmi.Namespace
	var ownerlessDataVolumeNames = dataVolumeNameObserved.Difference(dataVolumeNames).List()
	for _, dataVolumeName := range ownerlessDataVolumeNames {
		var dataVolume, err = h.dataVolumeCache.Get(dataVolumeNamespace, dataVolumeName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// NB(thxCode): ignores error, since this can't be fixed by an immediate requeue,
				// and also doesn't block the whole logic if the DataVolume has already been deleted.
				continue
			}
			return vmi, fmt.Errorf("failed to get DataVolume(%s/%s): %w", dataVolumeNamespace, dataVolumeName, err)
		}

		err = unsetBoundedDataVolumeReference(h.dataVolumeClient, dataVolume, vm)
		if err != nil {
			return vmi, fmt.Errorf("failed to revoke VitrualMachine(%s/%s) as DataVolume(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, dataVolumeNamespace, dataVolumeName, err)
		}
	}

	return vmi, nil
}

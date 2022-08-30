package virtualmachine

import (
	"fmt"
	"reflect"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/builder"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	VirtualMachineCreatorNodeDriver = "docker-machine-driver-harvester"
)

// hostLabelsReconcileMapping defines the mapping for reconciliation of node labels to virtual machine instance annotations
var hostLabelsReconcileMapping = []string{
	v1.LabelTopologyZone, v1.LabelTopologyRegion, v1.LabelHostname,
}

type VMIController struct {
	virtualMachineCache kubevirtctrl.VirtualMachineCache
	vmiClient           kubevirtctrl.VirtualMachineInstanceClient
	nodeCache           ctlcorev1.NodeCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	pvcCache            ctlcorev1.PersistentVolumeClaimCache
}

// UnsetOwnerOfPVCs erases the target VirtualMachine from the owner of the PVCs in annotation.
//
// When modifying the VirtualMachine's spec to remove the previously defined PVCs and recreating the VirtualMachineInstance,
// those previously defined PVCs still hold an OwnerReference of the VirtualMachine,
// but they are no longer used by the newly created VirtualMachineInstance.
//
// Since the handler of VMController has recorded the relationship in the PVC's annotation,
// this handler will erase the target owner from the PVC's annotation to avoid logic leak.
func (h *VMIController) UnsetOwnerOfPVCs(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
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
	var vmGVK = kubevirtv1.VirtualMachineGroupVersionKind
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

	var pvcNames = sets.String{}
	if vmiDesired := vm.Spec.Template; vmiDesired != nil { // just a defend, the validating webhook of virt-api will make this never happen
		pvcNames = getPVCNames(&vmiDesired.Spec)
	}
	var pvcNameObserved = getPVCNames(&vmi.Spec)

	// unsets ownerless PVCs
	var pvcNamespace = vmi.Namespace
	var ownerlessPVCNames = pvcNameObserved.Difference(pvcNames).List()
	for _, pvcName := range ownerlessPVCNames {
		var pvc, err = h.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// NB(thxCode): ignores error, since this can't be fixed by an immediate requeue,
				// and also doesn't block the whole logic if the PVC has already been deleted.
				continue
			}
			return vmi, fmt.Errorf("failed to get PVC(%s/%s): %w", pvcNamespace, pvcName, err)
		}

		err = unsetBoundedPVCReference(h.pvcClient, pvc, vm)
		if err != nil {
			return vmi, fmt.Errorf("failed to revoke VitrualMachine(%s/%s) as PVC(%s/%s)'s owner: %w",
				vm.Namespace, vm.Name, pvcNamespace, pvcName, err)
		}
	}

	return vmi, nil
}

// ReconcileFromHostLabels handles the propagation of metadata from node labels to VirtualMachineInstance annotations.
func (h *VMIController) ReconcileFromHostLabels(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	if creator := vmi.Labels[builder.LabelKeyVirtualMachineCreator]; creator != VirtualMachineCreatorNodeDriver {
		return vmi, nil
	}

	node, err := h.nodeCache.Get(vmi.Status.NodeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return vmi, nil
		}
		return vmi, fmt.Errorf("failed to get node %s for VirtualMachineInstance %s: %v", vmi.Status.NodeName, vmi.Name, err)
	}

	toUpdate := vmi.DeepCopy()
	for _, label := range hostLabelsReconcileMapping {
		srcValue, srcExists := node.Labels[label]
		if srcExists {
			if toUpdate.Annotations == nil {
				toUpdate.Annotations = map[string]string{}
			}
			toUpdate.Annotations[label] = srcValue
		} else if _, exist := toUpdate.Annotations[label]; exist {
			delete(toUpdate.Annotations, label)
		}
	}

	if !reflect.DeepEqual(toUpdate.Annotations, vmi.Annotations) {
		return h.vmiClient.Update(toUpdate)
	}

	return vmi, nil
}

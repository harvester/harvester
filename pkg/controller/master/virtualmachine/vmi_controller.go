package virtualmachine

import (
	"fmt"
	"reflect"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/builder"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	VirtualMachineCreatorNodeDriver = "docker-machine-driver-harvester"
)

// hostLabelsReconcileMapping defines the mapping for reconciliation of node labels to virtual machine instance annotations
var hostLabelsReconcileMapping = []string{
	corev1.LabelTopologyZone, corev1.LabelTopologyRegion, corev1.LabelHostname,
}

type VMIController struct {
	vmClient            kubevirtctrl.VirtualMachineClient
	virtualMachineCache kubevirtctrl.VirtualMachineCache
	vmiClient           kubevirtctrl.VirtualMachineInstanceClient
	nodeCache           ctlcorev1.NodeCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	recorder            record.EventRecorder
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
		} else {
			delete(toUpdate.Annotations, label)
		}
	}

	if !reflect.DeepEqual(toUpdate.Annotations, vmi.Annotations) {
		return h.vmiClient.Update(toUpdate)
	}

	return vmi, nil
}

func (h *VMIController) StopVMIfExceededQuota(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil || vmi.Status.Conditions == nil {
		return vmi, nil
	}

	for _, condition := range vmi.Status.Conditions {
		if condition.Type == kubevirtv1.VirtualMachineInstanceSynchronized &&
			condition.Status == corev1.ConditionFalse &&
			strings.Contains(condition.Message, "exceeded quota") {

			vm, err := h.virtualMachineCache.Get(vmi.Namespace, vmi.Name)
			if err != nil {
				return vmi, err
			}
			return vmi, h.stopVM(vm, condition.Message)
		}
	}
	return vmi, nil
}

func (h *VMIController) stopVM(vm *kubevirtv1.VirtualMachine, errMsg string) error {
	return stopVM(h.vmClient, h.recorder, vm, errMsg)
}

// removeDeprecatedFinalizer remove deprecated finalizer, so removing vm will not be blocked
func (h *VMIController) removeDeprecatedFinalizer(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil {
		return vmi, nil
	}
	vmiObj := vmi.DeepCopy()
	util.RemoveFinalizer(vmiObj, util.GetWranglerFinalizerName(deprecatedVMIUnsetOwnerOfPVCsFinalizer))
	if !reflect.DeepEqual(vmi, vmiObj) {
		return h.vmiClient.Update(vmiObj)
	}
	return vmi, nil
}

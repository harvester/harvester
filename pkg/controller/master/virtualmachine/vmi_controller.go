package virtualmachine

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/builder"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/indexeres"
)

const (
	VirtualMachineCreatorNodeDriver = "docker-machine-driver-harvester"
	HarvesterLabelPrefix            = "harvesterhci.io"
)

// hostLabelsReconcileMapping defines the mapping for reconciliation of node labels to virtual machine instance annotations
var hostLabelsReconcileMapping = []string{
	v1.LabelTopologyZone, v1.LabelTopologyRegion, v1.LabelHostname,
}

type VMIController struct {
	podClient           ctlcorev1.PodClient
	podCache            ctlcorev1.PodCache
	vmClient            kubevirtctrl.VirtualMachineClient
	virtualMachineCache kubevirtctrl.VirtualMachineCache
	vmiClient           kubevirtctrl.VirtualMachineInstanceClient
	nodeCache           ctlcorev1.NodeCache
	pvcClient           ctlcorev1.PersistentVolumeClaimClient
	recorder            record.EventRecorder
}

// Golang standard library's "maps" module contains the "Keys" function only for
// Golang <v1.21 and >= v1.23
// This function returns a sequence of keys of a map
func mapKeys(m map[apitypes.UID]string) []apitypes.UID {
	keys := make([]apitypes.UID, 0)
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// SyncHarvesterVMILabelsToPod ensures that all Harvester labels (i.e. those
// with the harvesterhci.io/ prefix) from the VirtualMachineInstance are synced
// to the Pod
func (h *VMIController) SyncHarvesterVMILabelsToPod(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	logrus.Debugf("Syncing labels %v for VMI %v to Pod", vmi.Labels, vmi.Name)

	harvesterVMILabels := map[string]string{}
	for label, value := range vmi.Labels {
		if strings.HasPrefix(label, HarvesterLabelPrefix) {
			harvesterVMILabels[label] = value
		}
	}

	activePodUIDs := vmi.Status.ActivePods
	if len(activePodUIDs) < 1 {
		logrus.Debugf("VMI %v does not have active Pods", vmi.Name)
		return vmi, nil
	}

	vmName, ok := vmi.Labels["harvesterhci.io/vmName"]
	if !ok {
		return vmi, fmt.Errorf("failed to determind VM name of VMI %v", vmi.Name)
	}

	pods, err := h.podCache.GetByIndex(indexeres.PodByVMNameIndex, ref.Construct(vmi.Namespace, vmName))
	if err != nil || len(pods) < 1 {
		return vmi, fmt.Errorf("failed to find pods for VMI %v, %v", vmi.Name, err)
	}

	for _, pod := range pods {
		// TODO: Replace with maps.Keys(activePodUIDs) after migrating to Golang
		// v1.23
		if !slices.Contains(mapKeys(activePodUIDs), pod.UID) {
			continue
		}

		newLabels := maps.Clone(pod.Labels)
		// delete Harvester labels from Pod, if they are deleted from VMI

		maps.DeleteFunc(newLabels, func(k, _ string) bool {
			if _, ok := harvesterVMILabels[k]; !ok && strings.HasPrefix(k, HarvesterLabelPrefix) {
				return true
			}
			return false
		})

		maps.Copy(newLabels, harvesterVMILabels)

		if !maps.Equal(pod.Labels, newLabels) {
			newPod := pod.DeepCopy()
			newPod.Labels = newLabels
			_, err := h.podClient.Update(newPod)
			if err != nil {
				return vmi, fmt.Errorf("failed to sync Harvester VMI labels to pod, %v", err)
			}
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

package node

import (
	"context"
	"fmt"
	"time"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/set"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlstoragev1 "github.com/harvester/harvester/pkg/generated/controllers/storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	nodeDownControllerName = "node-down-controller"
)

// nodeDownHandler force deletes VMI's pod when a node is down, so VMI can be reschduled to anothor healthy node
type nodeDownHandler struct {
	nodes                       ctlcorev1.NodeController
	nodeCache                   ctlcorev1.NodeCache
	pods                        ctlcorev1.PodClient
	pvcCache                    ctlcorev1.PersistentVolumeClaimCache
	vas                         ctlstoragev1.VolumeAttachmentClient
	vaCache                     ctlstoragev1.VolumeAttachmentCache
	virtualMachineInstanceCache v1.VirtualMachineInstanceCache
}

// DownRegister registers a controller to delete VMI when node is down
func DownRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	pods := management.CoreFactory.Core().V1().Pod()
	setting := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	vas := management.HarvesterStorageFactory.Storage().V1().VolumeAttachment()
	nodeDownHandler := &nodeDownHandler{
		nodes:                       nodes,
		nodeCache:                   nodes.Cache(),
		pods:                        pods,
		pvcCache:                    pvcs.Cache(),
		vas:                         vas,
		vaCache:                     vas.Cache(),
		virtualMachineInstanceCache: vmis.Cache(),
	}

	nodes.OnChange(ctx, nodeDownControllerName, nodeDownHandler.OnNodeChanged)
	setting.OnChange(ctx, nodeDownControllerName, nodeDownHandler.OnVMForceResetPolicyChanged)

	return nil
}

// OnNodeChanged monitors whether a node is ready or not
// Force delete a pod when all the below conditions are meet:
// 1. VMForceResetPolicy is enabled.
// 2. A node has been down for more than VMForceResetPolicy.Period seconds
// 3. The owner of Pod is VirtualMachineInstance.
// 4. The Pod is on a down node.
func (h *nodeDownHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	// get Ready condition
	cond := getNodeCondition(node.Status.Conditions, corev1.NodeReady)
	if cond == nil {
		return node, fmt.Errorf("can't find %s condition in node %s", corev1.NodeReady, node.Name)
	}

	// check whether node is healthy
	if cond.Status == corev1.ConditionTrue {
		return node, nil
	}

	// get VMForceResetPolicy setting
	vmForceResetPolicy, err := settings.DecodeVMForceResetPolicy(settings.VMForceResetPolicySet.Get())
	if err != nil {
		return node, err
	}

	if !vmForceResetPolicy.Enable {
		return node, nil
	}

	// if we haven't waited for vmForceResetPolicy.Period seconds, we enqueue event again
	if time.Since(cond.LastTransitionTime.Time) < time.Duration(vmForceResetPolicy.Period)*time.Second {
		deadline := cond.LastTransitionTime.Add(time.Duration(vmForceResetPolicy.Period) * time.Second)
		logrus.Debugf("Enqueue node event again at %v", deadline)
		h.nodes.EnqueueAfter(node.Name, time.Until(deadline))
		return node, nil
	}

	// get VMI pods on unhealthy node
	pods, err := h.pods.List(corev1.NamespaceAll, metav1.ListOptions{
		LabelSelector: labels.Set{
			kubevirtv1.AppLabel: "virt-launcher",
		}.String(),
		FieldSelector: "spec.nodeName=" + node.Name,
	})
	if err != nil {
		return node, err
	}

	var errs error
	pvcSet := set.New[string]()
	gracePeriod := int64(0)
	for i := range pods.Items {
		pod := pods.Items[i]
		logrus.Debugf("force delete pod %s/%s and VolumeAttachments", pod.Namespace, pod.Name)

		if err := h.pods.Delete(
			pod.Namespace,
			pod.Name,
			&metav1.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			}); err != nil {
			errs = multierr.Append(errs, fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err))
			continue
		}

		names, err := h.getPVCsName(&pod)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("failed to get PVCs name of pod %s/%s: %w", pod.Namespace, pod.Name, err))
			continue
		}
		pvcSet.Insert(names...)
	}

	if err := h.deleteVolumeAttachments(node.Name, pvcSet); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to delete VolumeAttachments: %w", err))
	}
	if errs != nil {
		return node, errs
	}

	return h.resetHeartbeat(node)
}

func (h *nodeDownHandler) OnVMForceResetPolicyChanged(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil ||
		setting.Name != settings.VMForceResetPolicySettingName || setting.Value == "" {
		return setting, nil
	}

	vmForceResetPolicy, err := settings.DecodeVMForceResetPolicy(setting.Value)
	if err != nil {
		return setting, err
	}

	if !vmForceResetPolicy.Enable {
		return setting, nil
	}

	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return setting, err
	}

	for _, node := range nodes {
		cond := getNodeCondition(node.Status.Conditions, corev1.NodeReady)
		if cond != nil && cond.Status != corev1.ConditionTrue {
			h.nodes.Enqueue(node.Name)
		}
	}
	return setting, nil
}

func getNodeCondition(conditions []corev1.NodeCondition, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	var cond *corev1.NodeCondition
	for i := range conditions {
		c := conditions[i]
		if c.Type == conditionType {
			cond = &c
			break
		}
	}
	return cond
}

func (h *nodeDownHandler) getPVCsName(pod *corev1.Pod) ([]string, error) {
	names := make([]string, 0)
	for _, vol := range pod.Spec.Volumes {
		if vol.VolumeSource.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := h.pvcCache.Get(pod.Namespace, vol.VolumeSource.PersistentVolumeClaim.ClaimName)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		names = append(names, pvc.Spec.VolumeName)
	}
	return names, nil
}

// deleteVolumeAttachments deletes all volume attachments of PVCs before post-host failure,
// due to the long controller-manager CSI processing period,
// causing the VM to restart to wait a long time.
// Then we delete VolumeAttachments directly.
// ref: https://github.com/harvester/harvester/issues/4049
func (h *nodeDownHandler) deleteVolumeAttachments(nodeName string, pvcSet set.Set[string]) error {
	if pvcSet.Len() == 0 {
		return nil
	}

	volumeAttachments, err := h.vaCache.List(labels.Everything())
	if err != nil {
		return err
	}
	for i := range volumeAttachments {
		va := volumeAttachments[i]

		if va.DeletionTimestamp != nil {
			continue
		}
		if va.Spec.NodeName != nodeName {
			continue
		}
		if va.Spec.Attacher != longhorntypes.LonghornDriverName {
			continue
		}
		if va.Spec.Source.PersistentVolumeName == nil {
			continue
		}
		if !pvcSet.Has(*va.Spec.Source.PersistentVolumeName) {
			continue
		}

		if err := h.vas.Delete(va.Name, &metav1.DeleteOptions{}); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}
		logrus.Infof("deleted volume attachment %v on downed node %v",
			va.Name,
			va.Spec.NodeName)
	}
	return nil
}

// resetHeartbeat force reduce node's heartbeat to immediately rebuild VMI,
// the NodeController in Kubevirt force rebuilds the VMI if the node's heartbeat has no update in 5 minutes(hardcode)
// Ensure that can take effect here by reducing 6 minutes
// ref: https://kubevirt.io/user-guide/operations/unresponsive_nodes/#virt-handler-heartbeat
func (h *nodeDownHandler) resetHeartbeat(node *corev1.Node) (*corev1.Node, error) {
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}

	if hb, ok := node.Annotations[kubevirtv1.VirtHandlerHeartbeat]; ok {
		hbt, err := time.Parse(time.RFC3339, hb)
		if err != nil {
			return node, err
		}
		if time.Since(hbt) > 5*time.Minute {
			return node, nil
		}
	}

	nodeCpy := node.DeepCopy()
	nodeCpy.Annotations[kubevirtv1.VirtHandlerHeartbeat] = metav1.Now().Add(-6 * time.Minute).Format(time.RFC3339)

	return h.nodes.Update(nodeCpy)
}

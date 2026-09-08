package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	maintainNodeControllerName = "maintain-node-controller"
	requeueDelay               = 10 * time.Second
	logMaxRemainingVMs         = 5
)

// maintainNodeHandler updates maintenance status of a node in its annotations, so that we can tell whether the node is
// entering maintenance mode(migrating VMs on it) or in maintenance mode(VMs migrated).
type maintainNodeHandler struct {
	nodes                       ctlcorev1.NodeClient
	nodeCache                   ctlcorev1.NodeCache
	virtualMachineClient        v1.VirtualMachineClient
	virtualMachineCache         v1.VirtualMachineCache
	virtualMachineInstanceCache v1.VirtualMachineInstanceCache
	enqueueAfter                func(string, time.Duration)
}

// MaintainRegister registers the node controller
func MaintainRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	maintainNodeHandler := &maintainNodeHandler{
		nodes:                       nodes,
		nodeCache:                   nodes.Cache(),
		virtualMachineClient:        vms,
		virtualMachineCache:         vms.Cache(),
		virtualMachineInstanceCache: vmis.Cache(),
		enqueueAfter:                nodes.EnqueueAfter,
	}

	nodes.OnChange(ctx, maintainNodeControllerName, maintainNodeHandler.OnNodeChanged)
	nodes.OnRemove(ctx, maintainNodeControllerName, maintainNodeHandler.OnNodeRemoved)

	return nil
}

// OnNodeChanged updates node maintenance status when all VMs are migrated
func (h *maintainNodeHandler) OnNodeChanged(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}
	condition := util.GetMaintenanceModeCondition(node)

	if !util.IsMaintenanceModeCondition(condition, corev1.ConditionTrue, util.NodeConditionReasonEvacuating) {
		return node, nil
	}

	// Wait until no VMs are running on that node.
	vmiList, err := virtualmachineinstance.ListByNode(node, labels.NewSelector(), h.virtualMachineInstanceCache)
	if err != nil {
		return node, err
	}

	if len(vmiList) != 0 {
		// Get the names of the remaining VMs, but limit the number of
		// names logged to avoid excessive log output.
		vmNames := make([]string, 0, logMaxRemainingVMs)
		for i, vmi := range vmiList {
			if i < logMaxRemainingVMs {
				vmNames = append(vmNames, util.GetNamespacedName(vmi))
			} else {
				vmNames = append(vmNames, fmt.Sprintf("... and %d more", len(vmiList)-logMaxRemainingVMs))
				break
			}
		}

		logrus.WithFields(logrus.Fields{
			"node":         node.Name,
			"remainingVMs": len(vmiList),
			"vms":          strings.Join(vmNames, ", "),
		}).Info("Waiting for VMs to leave node before completing maintenance mode")

		if h.enqueueAfter != nil {
			h.enqueueAfter(node.Name, requeueDelay)
		}

		return node, nil
	}

	// Restart those VMs that have been labeled to be shut down before
	// maintenance mode and that should be restarted when the node has
	// successfully switched into maintenance mode.
	if err := util.RestartMaintenanceModeVMsOnNode(h.virtualMachineClient, h.virtualMachineCache, node.Name, util.MaintainModeStrategyShutdownAndRestartAfterEnable); err != nil {
		return node, err
	}

	return util.TransitionMaintenanceModeCondition(h.nodes, node.Name,
		corev1.ConditionTrue, util.NodeConditionReasonEvacuating,
		corev1.ConditionTrue, util.NodeConditionReasonCompleted, util.MaintenanceModeMessageCompleted)
}

// OnNodeRemoved Ensure that all "harvesterhci.io/maintain-mode-strategy-node-name"
// annotations on VMs are removed that are referencing this node.
func (h *maintainNodeHandler) OnNodeRemoved(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp == nil {
		return node, nil
	}

	if !util.IsMaintenanceModeEngaged(node) {
		return node, nil
	}

	vms, err := h.virtualMachineCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return node, fmt.Errorf("failed to list VMs: %w", err)
	}

	for _, vm := range vms {
		if vm.Annotations == nil || vm.Annotations[util.AnnotationMaintainModeStrategyNodeName] != node.Name {
			continue
		}
		vmCopy := vm.DeepCopy()
		delete(vmCopy.Annotations, util.AnnotationMaintainModeStrategyNodeName)
		_, err = h.virtualMachineClient.Update(vmCopy)
		if err != nil {
			return node, err
		}
	}

	return node, nil
}

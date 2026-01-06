package node

import (
	"context"
	"fmt"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	maintainNodeControllerName        = "maintain-node-controller"
	maintainErrorRetentionTimeSeconds = 120
	MaintainStatusAnnotationKey       = "harvesterhci.io/maintain-status"
	MaintainStatusComplete            = "completed"
	MaintainStatusRunning             = "running"
)

// maintainNodeHandler updates the maintenance status of a node in its annotations, so that we can tell whether the node is
// entering maintenance mode (migrating VMs on it) or in maintenance mode(VMs migrated).
type maintainNodeHandler struct {
	nodes                       ctlcorev1.NodeController
	nodeCache                   ctlcorev1.NodeCache
	virtualMachineClient        v1.VirtualMachineClient
	virtualMachineCache         v1.VirtualMachineCache
	virtualMachineInstanceCache v1.VirtualMachineInstanceCache
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

	if maintenanceStatus, ok := node.Annotations[MaintainStatusAnnotationKey]; !ok || maintenanceStatus != MaintainStatusRunning {
		// Make sure the maintenance mode status condition is cleaned up when
		// there is no "harvesterhci.io/maintain-status" annotation (e.g., the
		// maintenance mode has been disabled).

		// If it is an error status condition, then reconcile again for a short
		// period (~ 2 minute) so that there is some time to display the error
		// on the `Hosts` page (Wrangler collects such status conditions and
		// the UI displays them). If the status is older than the configured
		// retention time, proceed with the cleanup.
		condition := util.GetConditionFromNode(node, util.NodeConditionTypeMaintenanceMode)
		if condition != nil && condition.Reason == util.NodeConditionReasonError {
			retentionTime := time.Duration(maintainErrorRetentionTimeSeconds) * time.Second
			if time.Since(condition.LastHeartbeatTime.Time) <= retentionTime {
				h.nodes.EnqueueAfter(node.Name, 5*time.Second)
				return node, nil
			}
		}

		// Clean up the maintenance mode status condition if the annotation is
		// not present; otherwise leave it as is.
		if !ok {
			nodeCopy := node.DeepCopy()
			removed := util.RemoveConditionFromNode(nodeCopy, util.NodeConditionTypeMaintenanceMode)
			if removed {
				if _, err := h.nodes.UpdateStatus(nodeCopy); err != nil {
					return node, fmt.Errorf("failed to clean up %q condition: %w", util.NodeConditionTypeMaintenanceMode, err)
				}
				return nodeCopy, nil
			}
		}

		return node, nil
	}

	// Wait until no VMs are running on that node.
	vmiList, err := virtualmachineinstance.ListByNode(node, labels.NewSelector(), h.virtualMachineInstanceCache)
	if err != nil {
		return node, err
	}
	if len(vmiList) != 0 {
		return node, nil
	}

	// Restart VMs that were shut down for maintenance and should be restarted.
	if err = h.restartShutdownVMs(node); err != nil {
		return node, err
	}

	nodeCopy := node.DeepCopy()
	nodeCopy.Annotations[MaintainStatusAnnotationKey] = MaintainStatusComplete
	nodeCopy, err = h.nodes.Update(nodeCopy)
	if err != nil {
		return node, err
	}

	util.AddOrUpdateConditionToNode(nodeCopy, corev1.NodeCondition{
		Type:               util.NodeConditionTypeMaintenanceMode,
		Status:             corev1.ConditionTrue,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             util.NodeConditionReasonCompleted,
		Message:            "Maintenance mode enabled",
	})

	return h.nodes.UpdateStatus(nodeCopy)
}

// OnNodeRemoved Ensure that all "harvesterhci.io/maintain-mode-strategy-node-name"
// annotations on VMs are removed that are referencing this node.
func (h *maintainNodeHandler) OnNodeRemoved(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp == nil || node.Annotations == nil {
		return node, nil
	}

	if _, ok := node.Annotations[MaintainStatusAnnotationKey]; !ok {
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

// restartShutdownVMs restarts those VMs that have been labeled to be shut down
// before maintenance mode, and that should be restarted when the node has
// successfully switched into maintenance mode.
func (h *maintainNodeHandler) restartShutdownVMs(node *corev1.Node) error {
	selector := labels.Set{util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterEnable}.AsSelector()
	vmList, err := h.virtualMachineCache.List(node.Namespace, selector)
	if err != nil {
		return fmt.Errorf("failed to list VMs with labels %s: %w", selector.String(), err)
	}

	for _, vm := range vmList {
		// Make sure that this VM was shut down as part of the maintenance
		// mode of the given node.
		if vm.Annotations == nil || vm.Annotations[util.AnnotationMaintainModeStrategyNodeName] != node.Name {
			continue
		}

		logrus.WithFields(logrus.Fields{
			"namespace":           vm.Namespace,
			"virtualmachine_name": vm.Name,
		}).Info("Restarting the VM that was temporarily shut down for maintenance mode")

		// Update the run strategy of the VM to start it and remove the
		// annotation that was previously set when the node went into
		// maintenance mode.
		// Get the running strategy that is stored in the annotation of the
		// VM when it is shut down. Note, in general this is automatically
		// patched by the VM mutator in general.
		runStrategy := kubevirtv1.VirtualMachineRunStrategy(vm.Annotations[util.AnnotationRunStrategy])
		if runStrategy == "" {
			runStrategy = kubevirtv1.RunStrategyRerunOnFailure
		}

		vmCopy := vm.DeepCopy()
		vmCopy.Spec.RunStrategy = &[]kubevirtv1.VirtualMachineRunStrategy{runStrategy}[0]
		delete(vmCopy.Annotations, util.AnnotationMaintainModeStrategyNodeName)

		_, err = h.virtualMachineClient.Update(vmCopy)
		if err != nil {
			return fmt.Errorf("failed to start VM %s/%s: %w", vm.Namespace, vm.Name, err)
		}
	}

	return nil
}

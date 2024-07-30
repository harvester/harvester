package node

import (
	"context"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	maintainNodeControllerName  = "maintain-node-controller"
	MaintainStatusAnnotationKey = "harvesterhci.io/maintain-status"
	MaintainStatusComplete      = "completed"
	MaintainStatusRunning       = "running"
)

// maintainNodeHandler updates maintenance status of a node in its annotations, so that we can tell whether the node is
// entering maintenance mode(migrating VMs on it) or in maintenance mode(VMs migrated).
type maintainNodeHandler struct {
	nodes                       ctlcorev1.NodeClient
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

	// Restart those VMs that have been labeled to be shut down before
	// maintenance mode and that should be restarted when the node has
	// successfully switched into maintenance mode.
	selector := labels.Set{util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterEnable}.AsSelector()
	vmList, err := h.virtualMachineCache.List(node.Namespace, selector)
	if err != nil {
		return node, fmt.Errorf("failed to list VMs with labels %s: %w", selector.String(), err)
	}
	for _, vm := range vmList {
		// Make sure that this VM was shut down as part of the maintenance
		// mode of the given node.
		if vm.Annotations[util.AnnotationMaintainModeStrategyNodeName] != node.Name {
			continue
		}

		logrus.WithFields(logrus.Fields{
			"namespace":           vm.Namespace,
			"virtualmachine_name": vm.Name,
		}).Info("restarting the VM that was temporary shut down for maintenance mode")

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
			return node, fmt.Errorf("failed to start VM %s/%s: %w", vm.Namespace, vm.Name, err)
		}
	}

	toUpdate := node.DeepCopy()
	toUpdate.Annotations[MaintainStatusAnnotationKey] = MaintainStatusComplete
	return h.nodes.Update(toUpdate)
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

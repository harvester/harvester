package node

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/harvester/harvester/pkg/config"
	v1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	maintainNodeControllerName = "maintain-node-controller"
	labelNodeNameKey           = "kubevirt.io/nodeName"

	MaintainStatusAnnotationKey = "harvesterhci.io/maintain-status"
	MaintainStatusComplete      = "completed"
	MaintainStatusRunning       = "running"
)

// maintainNodeHandler updates maintenance status of a node in its annotations, so that we can tell whether the node is
// entering maintenance mode(migrating VMs on it) or in maintenance mode(VMs migrated).
type maintainNodeHandler struct {
	nodes                       ctlcorev1.NodeClient
	nodeCache                   ctlcorev1.NodeCache
	virtualMachineInstanceCache v1.VirtualMachineInstanceCache
}

// MaintainRegister registers the node controller
func MaintainRegister(ctx context.Context, management *config.Management, options config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	maintainNodeHandler := &maintainNodeHandler{
		nodes:                       nodes,
		nodeCache:                   nodes.Cache(),
		virtualMachineInstanceCache: vmis.Cache(),
	}

	nodes.OnChange(ctx, maintainNodeControllerName, maintainNodeHandler.OnNodeChanged)

	return nil
}

// OnNodeChanged updates node maintenance status when all VMs are migrated
func (h *maintainNodeHandler) OnNodeChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}
	if maintenanceStatus, ok := node.Annotations[MaintainStatusAnnotationKey]; !ok || maintenanceStatus != MaintainStatusRunning {
		return node, nil
	}
	sets := labels.Set{
		labelNodeNameKey: node.Name,
	}
	vmis, err := h.virtualMachineInstanceCache.List(corev1.NamespaceAll, sets.AsSelector())
	if err != nil {
		return node, err
	}
	if len(vmis) != 0 {
		return node, nil
	}
	toUpdate := node.DeepCopy()
	toUpdate.Annotations[MaintainStatusAnnotationKey] = MaintainStatusComplete
	return h.nodes.Update(toUpdate)
}

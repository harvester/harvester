package node

import (
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	ctlnode "github.com/harvester/harvester/pkg/controller/master/node"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(nodeCache v1.NodeCache) types.Validator {
	return &nodeValidator{
		nodeCache: nodeCache,
	}
}

type nodeValidator struct {
	types.DefaultValidator
	nodeCache v1.NodeCache
}

func (v *nodeValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"nodes"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Node{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Update,
		},
	}
}

func (v *nodeValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)

	nodeList, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return err
	}

	return validateCordonAndMaintenanceMode(oldNode, newNode, nodeList)
}

func validateCordonAndMaintenanceMode(oldNode, newNode *corev1.Node, nodeList []*corev1.Node) error {
	// if old node already have "maintain-status" annotation or Unscheduleable=true,
	// it has already been enabled, so we skip it
	if _, ok := oldNode.Annotations[ctlnode.MaintainStatusAnnotationKey]; ok || oldNode.Spec.Unschedulable {
		return nil
	}
	// if new node doesn't have "maintain-status" annotation and Unscheduleable=false, we skip it
	if _, ok := newNode.Annotations[ctlnode.MaintainStatusAnnotationKey]; !ok && !newNode.Spec.Unschedulable {
		return nil
	}

	for _, node := range nodeList {
		if node.Name == oldNode.Name {
			continue
		}

		// Return when we find another available node
		if _, ok := node.Annotations[ctlnode.MaintainStatusAnnotationKey]; !ok && !node.Spec.Unschedulable {
			return nil
		}
	}
	return werror.NewBadRequest("can't enable maintenance mode or cordon on the last available node")
}

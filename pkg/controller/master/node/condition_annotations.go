package node

import (
	"context"
	"reflect"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	NodeConditionAnnotationHandler = "node-condition-annotation-handler"
)

type ConditionManager struct {
	nodeClient ctlcorev1.NodeClient
}

func ConditionAnnotationRegister(ctx context.Context, management *config.Management, options config.Options) error {

	nodeCtl := management.CoreFactory.Core().V1().Node()
	cm := &ConditionManager{
		nodeClient: nodeCtl,
	}
	nodeCtl.OnChange(ctx, NodeConditionAnnotationHandler, cm.UpdateStatusAnnotations)
	return nil
}

// UpdateStatusAnnotations will reconcile node conditions and apply a condition and message annotation
// to the node object, which can be used by UI to render more informative UI messages
func (cm *ConditionManager) UpdateStatusAnnotations(_ string, node *corev1.Node) (*corev1.Node, error) {
	nodeCopy := node.DeepCopy()
	cond := getNodeCondition(nodeCopy.Status.Conditions, HarvesterNodeCondDrained)
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	if cond == nil || cond.Status == corev1.ConditionFalse {
		// condition not found, ensure that the annotation is cleared if needed
		delete(nodeCopy.Annotations, util.HarvesterReportedConditionKey)
		delete(nodeCopy.Annotations, util.HarvesterReportedConditionMessageKey)
	}
	if cond != nil && cond.Status == corev1.ConditionTrue {
		nodeCopy.Annotations[util.HarvesterReportedConditionKey] = string(HarvesterNodeCondDrained)
		nodeCopy.Annotations[util.HarvesterReportedConditionMessageKey] = cond.Message
	}

	// update annotations as needed
	if !reflect.DeepEqual(node.Annotations, nodeCopy.Annotations) {
		return cm.nodeClient.Update(nodeCopy)
	}

	return node, nil
}

package node

import (
	"context"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/harvester/harvester/pkg/config"
	ctlclusterv1 "github.com/harvester/harvester/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	nodeRemoveControllerName = "node-remove-controller"
)

// nodeRemoveHandler force remove machine
type nodeRemoveHandler struct {
	machineClient ctlclusterv1.MachineClient
}

// RemoveRegister registers a controller to remove machine when node is removed
func RemoveRegister(ctx context.Context, management *config.Management, _ config.Options) error {
	nodes := management.CoreFactory.Core().V1().Node()
	machineClient := management.ClusterFactory.Cluster().V1beta1().Machine()
	handler := &nodeRemoveHandler{
		machineClient: machineClient,
	}

	nodes.OnRemove(ctx, nodeRemoveControllerName, handler.OnNodeRemoved)

	return nil
}

func (h *nodeRemoveHandler) OnNodeRemoved(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp == nil || node.Annotations == nil || node.Annotations[clusterv1.MachineAnnotation] == "" {
		return node, nil
	}

	_, err := h.machineClient.Get(util.FleetLocalNamespaceName, node.Annotations[clusterv1.MachineAnnotation], metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		logrus.WithError(err).WithFields(logrus.Fields{
			"node":    node.Name,
			"machine": node.Annotations[clusterv1.MachineAnnotation],
		}).Error("Can't get machine")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"node":    node.Name,
		"machine": node.Annotations[clusterv1.MachineAnnotation],
	}).Info("Attempting to delete machine")
	if err := h.machineClient.Delete(util.FleetLocalNamespaceName, node.Annotations[clusterv1.MachineAnnotation], &metav1.DeleteOptions{}); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"node":    node.Name,
			"machine": node.Annotations[clusterv1.MachineAnnotation],
		}).Error("failed to delete machine")
		return nil, err
	}
	return nil, nil
}

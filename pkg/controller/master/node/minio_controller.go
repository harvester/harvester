package node

import (
	"context"
	"time"

	appsv1 "github.com/rancher/wrangler-api/pkg/generated/controllers/apps/v1"
	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester/pkg/config"
)

const (
	controllerAgentName = "balance-minio-node-controller"
	appLabelKey         = "app"
	minioName           = "minio"
	timestampAnnoKey    = "cattle.io/timestamp"
)

var (
	throttleDelay = 1 * time.Minute
)

// Register registers the node controller
func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	pods := management.CoreFactory.Core().V1().Pod()
	statefulsets := management.AppsFactory.Apps().V1().StatefulSet()
	controller := &Handler{
		podCache:         pods.Cache(),
		statefulSets:     statefulsets,
		statefulSetCache: statefulsets.Cache(),
	}

	nodes.OnChange(ctx, controllerAgentName, controller.OnChanged)
	return nil
}

// Handler balances minio pods if applicable when new nodes join
type Handler struct {
	podCache         v1.PodCache
	statefulSets     appsv1.StatefulSetClient
	statefulSetCache appsv1.StatefulSetCache
}

// OnChanged tries to make minio pods balanced if they are not
func (h *Handler) OnChanged(key string, node *apiv1.Node) (*apiv1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return node, nil
	}

	for _, c := range node.Status.Conditions {
		if c.Type == apiv1.NodeReady && c.Status != apiv1.ConditionTrue {
			// skip unready nodes
			return node, nil
		}

		if c.Type != apiv1.NodeReady && c.Status == apiv1.ConditionTrue {
			// skip deploy minio to node with conditions like nodeMemoryPressure, nodeDiskPressure, nodePIDPressure
			// and nodeNetworkUnavailable equal to true
			return node, nil
		}
	}

	if len(node.Spec.Taints) > 0 {
		// skip taints nodes
		return node, nil
	}

	sets := labels.Set{
		appLabelKey: minioName,
	}
	pods, err := h.podCache.List(config.Namespace, sets.AsSelector())
	if err != nil {
		return nil, err
	}
	if len(pods) < 4 {
		// only take care of distributed minio
		return node, nil
	}

	var nodeSet = make(map[string]bool)
	for _, p := range pods {
		if p.Status.Phase != apiv1.PodRunning {
			// proceed when all pods are running
			return node, nil
		}
		if p.Spec.NodeName == node.Name {
			// The node is already running minio pods
			return node, nil
		}
		nodeSet[p.Spec.NodeName] = true
	}
	if len(nodeSet) < 3 {
		// balance minio pods to tolerate node disruption
		if err := h.redeployMinio(); err != nil {
			return node, err
		}
	}
	return node, nil
}

func (h *Handler) redeployMinio() error {
	ss, err := h.statefulSetCache.Get(config.Namespace, minioName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	toUpdate := ss.DeepCopy()
	if toUpdate.Spec.Template.Annotations == nil {
		toUpdate.Spec.Template.Annotations = make(map[string]string, 1)
	}
	prevTimestamp := toUpdate.Spec.Template.Annotations[timestampAnnoKey]
	if prevTimestamp != "" {
		prevTime, err := time.Parse(time.RFC3339, prevTimestamp)
		if err != nil {
			return err
		}
		if prevTime.Add(throttleDelay).After(time.Now()) {
			return nil
		}
	}
	toUpdate.Spec.Template.Annotations[timestampAnnoKey] = time.Now().Format(time.RFC3339)
	_, err = h.statefulSets.Update(toUpdate)
	return err
}

package upgrade

import (
	"strconv"
	"time"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradectlv1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
	"github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/rand"
)

// planHandler syncs on plan completions
// When a plan completes, it set the NodesPrepared condition of upgrade CRD to be true.
type planHandler struct {
	namespace        string
	upgradeClient    ctlharvesterv1.UpgradeClient
	upgradeCache     ctlharvesterv1.UpgradeCache
	nodeCache        v1.NodeCache
	planClient       upgradectlv1.PlanClient
	planEnqueueAfter func(planNamespace, planName string, timeout time.Duration)
}

func (h *planHandler) OnChanged(_ string, plan *upgradev1.Plan) (*upgradev1.Plan, error) {
	if plan == nil || plan.DeletionTimestamp != nil {
		return plan, nil
	}

	if plan.Labels == nil || plan.Labels[harvesterUpgradeLabel] == "" || plan.Spec.NodeSelector == nil {
		return plan, nil
	}

	planLogrus := logrus.WithFields(logrus.Fields{
		"plan":      plan.Name,
		"namespace": plan.Namespace,
	})

	nodesAtLatestHash, err := allAtLatestHash(h.nodeCache, plan)
	if err != nil {
		return plan, err
	}
	if !nodesAtLatestHash {
		// Add jitter to avoid multiple plans re-enqueuing simultaneously
		planLogrus.Info("Nodes pending hash update, requeuing")
		jitter := time.Duration(rand.Intn(5)) * time.Second
		h.planEnqueueAfter(plan.Namespace, plan.Name, upgradeCommonRequeueInterval+jitter)
		return plan, nil
	}

	planLogrus.Info("All nodes at latest hash")
	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	// All nodes for a plan are done at this stage
	upgradeName, ok := plan.Labels[harvesterUpgradeLabel]
	if !ok {
		return plan, nil
	}
	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if errors.IsNotFound(err) {
		return plan, nil
	} else if err != nil {
		return plan, err
	}

	component := plan.Labels[harvesterUpgradeComponentLabel]
	componentAnnotations := map[string]string{
		skipManifestsApplyComponent:  skipManifestsApplyPlanCompletedAnnotation,
		skipManifestsRemoveComponent: skipManifestsRemovePlanCompletedAnnotation,
		cleanupComponent:             imageCleanupPlanCompletedAnnotation,
	}
	if annotation, ok := componentAnnotations[component]; ok {
		toUpdate := upgrade.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = make(map[string]string)
		}
		toUpdate.Annotations[annotation] = strconv.FormatBool(true)
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return plan, err
		}
	}
	if !harvesterv1.NodesPrepared.IsTrue(upgrade) && component == nodeComponent {
		toUpdate := upgrade.DeepCopy()
		setNodesPreparedCondition(toUpdate, corev1.ConditionTrue, "", "")
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return plan, err
		}
	}

	return plan, nil
}

// allAtLatestHash returns true if all nodes matching the plan's NodeSelector
// are updated to plan.Status.LatestHash (or are disabled), and returns false if any node is still pending.
func allAtLatestHash(nodeCache v1.NodeCache, plan *upgradev1.Plan) (bool, error) {
	requirementPlanNotLatest, err := labels.NewRequirement(upgrade.LabelPlanName(plan.Name), selection.NotIn, []string{"disabled", plan.Status.LatestHash})
	if err != nil {
		return false, err
	}
	selector, err := metav1.LabelSelectorAsSelector(plan.Spec.NodeSelector)
	if err != nil {
		return false, err
	}
	selector = selector.Add(*requirementPlanNotLatest)
	nodes, err := nodeCache.List(selector)
	if err != nil {
		return false, err
	}

	return len(nodes) == 0, nil
}

package upgrade

import (
	"github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradectlv1 "github.com/rancher/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

// planHandler syncs on plan completions
// When a server plan completes, it creates a relevant agent plan.
// When a agent plan completes, it set the NodesUpgraded condition of upgrade CRD to be true.
type planHandler struct {
	namespace     string
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
	nodeCache     v1.NodeCache
	planClient    upgradectlv1.PlanClient
}

func (h *planHandler) OnChanged(key string, plan *upgradev1.Plan) (*upgradev1.Plan, error) {
	if plan == nil || plan.DeletionTimestamp != nil {
		return plan, nil
	}

	if plan.Labels == nil || plan.Labels[harvesterUpgradeLabel] == "" || plan.Spec.NodeSelector == nil {
		return plan, nil
	}

	requirementPlanNotLatest, err := labels.NewRequirement(upgrade.LabelPlanName(plan.Name), selection.NotIn, []string{"disabled", plan.Status.LatestHash})
	if err != nil {
		return plan, err
	}
	selector, err := metav1.LabelSelectorAsSelector(plan.Spec.NodeSelector)
	if err != nil {
		return plan, err
	}
	selector = selector.Add(*requirementPlanNotLatest)
	nodes, err := h.nodeCache.List(selector)
	if err != nil {
		return plan, err
	}
	if len(nodes) != 0 {
		return plan, nil
	}

	// All nodes are upgraded at this stage
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
	if component == serverComponent {
		// server nodes are upgraded, now create agent plan to upgrade agent nodes.
		agentPlan := agentPlan(upgrade)
		if _, err := h.planClient.Create(agentPlan); err != nil && !errors.IsAlreadyExists(err) {
			return plan, err
		}
	} else if !harvesterv1.NodesUpgraded.IsTrue(upgrade) && component == agentComponent {
		// all nodes are upgraded
		toUpdate := upgrade.DeepCopy()
		setNodesUpgradedCondition(toUpdate, corev1.ConditionTrue, "", "")
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

package upgrade

import (
	"github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradectlv1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

// planHandler syncs on plan completions
// When a plan completes, it set the NodesPrepared condition of upgrade CRD to be true.
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

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

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
	if !harvesterv1.NodesPrepared.IsTrue(upgrade) && component == nodeComponent {
		toUpdate := upgrade.DeepCopy()
		setNodesPreparedCondition(toUpdate, corev1.ConditionTrue, "", "")
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return plan, err
		}
	}

	return plan, nil
}

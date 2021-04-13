package upgrade

import (
	"reflect"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradev1 "github.com/rancher/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

const (
	stateUpgrading     = "Upgrading"
	stateSucceeded     = "Succeeded"
	stateFailed        = "Failed"
	helmChartLabel     = "helmcharts.helm.cattle.io/chart"
	upgradePlanLabel   = "upgrade.cattle.io/plan"
	upgradeNodeLabel   = "upgrade.cattle.io/node"
	upgradeStateLabel  = "harvesterhci.io/upgradeState"
	harvesterChartname = "harvester"
)

// jobHandler syncs upgrade CRD status on upgrade job changes
type jobHandler struct {
	namespace     string
	planCache     upgradev1.PlanCache
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
}

func (h *jobHandler) OnChanged(key string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.DeletionTimestamp != nil || job.Labels == nil {
		return job, nil
	}

	chartName := job.Labels[helmChartLabel]
	planName := job.Labels[upgradePlanLabel]
	nodeName := job.Labels[upgradeNodeLabel]
	if chartName == harvesterChartname {
		return h.syncHelmChartJob(job)
	} else if planName != "" && nodeName != "" {
		return h.syncNodeJob(job, planName, nodeName)
	}

	return job, nil
}

func (h *jobHandler) syncNodeJob(job *batchv1.Job, planName string, nodeName string) (*batchv1.Job, error) {
	plan, err := h.planCache.Get(k3osSystemNamespace, planName)
	if err != nil {
		return job, err
	}
	upgradeName, ok := plan.Labels[harvesterUpgradeLabel]
	if !ok {
		return job, nil
	}
	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if err != nil {
		return job, err
	}
	toUpdate := upgrade.DeepCopy()

	if job.Status.Active > 0 {
		setNodeUpgradeStatus(toUpdate, nodeName, stateUpgrading, "", "")
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == "True" {
			setNodeUpgradeStatus(toUpdate, nodeName, stateFailed, condition.Reason, condition.Message)
		} else if condition.Type == batchv1.JobComplete && condition.Status == "True" {
			setNodeUpgradeStatus(toUpdate, nodeName, stateSucceeded, condition.Reason, condition.Message)
		}
	}
	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	return job, nil
}

func (h *jobHandler) syncHelmChartJob(job *batchv1.Job) (*batchv1.Job, error) {
	sets := labels.Set{
		harvesterLatestUpgradeLabel: "true",
	}
	onGoingUpgrades, err := h.upgradeCache.List(h.namespace, sets.AsSelector())
	if err != nil {
		return job, err
	}
	if len(onGoingUpgrades) == 0 {
		return job, nil
	}
	currentUpgrade := onGoingUpgrades[0]
	toUpdate := currentUpgrade.DeepCopy()

	if !harvesterv1.SystemServicesUpgraded.IsUnknown(currentUpgrade) || job.Status.Active > 0 {
		return job, nil
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == "True" {
			setHelmChartUpgradeStatus(toUpdate, v1.ConditionFalse, condition.Reason, condition.Message)
		} else if condition.Type == batchv1.JobComplete && condition.Status == "True" {
			setHelmChartUpgradeStatus(toUpdate, v1.ConditionTrue, condition.Reason, condition.Message)
		}
	}
	if !reflect.DeepEqual(currentUpgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return job, err
		}
	}

	return job, nil
}

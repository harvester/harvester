package upgrade

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

// podHandler syncs upgrade CRD status on upgrade pod status changes
type podHandler struct {
	namespace     string
	planCache     upgradev1.PlanCache
	upgradeClient v1alpha1.UpgradeClient
	upgradeCache  v1alpha1.UpgradeCache
}

func (h *podHandler) OnChanged(key string, pod *v1.Pod) (*v1.Pod, error) {
	if pod == nil || pod.DeletionTimestamp != nil || pod.Labels == nil {
		return pod, nil
	}

	chartName := pod.Labels[helmChartLabel]
	planName := pod.Labels[upgradePlanLabel]
	nodeName := pod.Labels[upgradeNodeLabel]
	if chartName == harvesterChartname {
		return h.syncHelmChartPod(pod)
	} else if planName != "" && nodeName != "" {
		return h.syncNodeUpgradePod(pod, planName, nodeName)
	}
	return pod, nil
}

func (h *podHandler) syncHelmChartPod(pod *v1.Pod) (*v1.Pod, error) {
	if pod.Status.Phase == v1.PodSucceeded {
		return pod, nil
	}

	sets := labels.Set{
		harvesterLatestUpgradeLabel: "true",
	}
	onGoingUpgrades, err := h.upgradeCache.List(h.namespace, sets.AsSelector())
	if err != nil {
		return pod, err
	}
	if len(onGoingUpgrades) == 0 {
		return pod, nil
	}
	upgrade := onGoingUpgrades[0]
	if !apisv1alpha1.SystemServicesUpgraded.IsUnknown(upgrade) {
		return pod, nil
	}
	toUpdate := upgrade.DeepCopy()

	reason, message := getPodWaitingStatus(pod)
	setHelmChartUpgradeStatus(toUpdate, v1.ConditionUnknown, reason, message)

	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return pod, err
		}
	}
	return pod, nil
}

func (h *podHandler) syncNodeUpgradePod(pod *v1.Pod, planName string, nodeName string) (*v1.Pod, error) {
	if pod.Status.Phase == v1.PodSucceeded {
		return pod, nil
	}
	plan, err := h.planCache.Get(k3osSystemNamespace, planName)
	if err != nil {
		return pod, err
	}
	upgradeName, ok := plan.Labels[harvesterUpgradeLabel]
	if !ok {
		return pod, nil
	}
	upgrade, err := h.upgradeCache.Get(h.namespace, upgradeName)
	if err != nil {
		return pod, err
	}
	toUpdate := upgrade.DeepCopy()

	reason, message := getPodWaitingStatus(pod)
	setNodeUpgradeStatus(toUpdate, nodeName, stateUpgrading, reason, message)
	if !reflect.DeepEqual(upgrade, toUpdate) {
		if _, err := h.upgradeClient.Update(toUpdate); err != nil {
			return pod, err
		}
	}

	return pod, nil
}

func getPodWaitingStatus(pod *v1.Pod) (reason string, message string) {
	var containerStatuses []v1.ContainerStatus
	containerStatuses = append(containerStatuses, pod.Status.InitContainerStatuses...)
	containerStatuses = append(containerStatuses, pod.Status.ContainerStatuses...)

	for _, status := range containerStatuses {
		if status.State.Waiting != nil && len(status.State.Waiting.Reason) > 0 && status.State.Waiting.Reason != "PodInitializing" {
			reason = status.State.Waiting.Reason
			message = status.State.Waiting.Message
			return
		}
	}
	return
}

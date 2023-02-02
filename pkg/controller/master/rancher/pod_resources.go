package rancher

import (
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/harvester/harvester/pkg/indexeres"
)

// Since multi-cluster-management-agent is disabled in the Harvester embedded Rancher,
// copy and modified this file from Rancher to fix https://github.com/harvester/harvester/issues/2310
// source file: https://github.com/rancher/rancher/tree/release/v2.6/pkg/controllers/managementagent/podresources

const (
	RequestsAnnotation = "management.cattle.io/pod-requests"
	LimitsAnnotation   = "management.cattle.io/pod-limits"
)

func (h *Handler) PodResourcesOnChanged(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil {
		return node, nil
	}

	h.nodeController.EnqueueAfter(node.Name, 15*time.Second)

	pods, err := h.getNonTerminatedPods(node)
	if err != nil {
		return nil, err
	}

	requests, limits, err := getPodResourceAnnotations(pods)
	if err != nil {
		return nil, err
	}

	if node.Annotations[RequestsAnnotation] != requests ||
		node.Annotations[LimitsAnnotation] != limits {
		toUpdate := node.DeepCopy()
		if toUpdate.Annotations == nil {
			toUpdate.Annotations = map[string]string{}
		}
		toUpdate.Annotations[RequestsAnnotation] = requests
		toUpdate.Annotations[LimitsAnnotation] = limits
		return h.nodeController.Update(toUpdate)
	}

	return node, nil
}

func (h *Handler) getNonTerminatedPods(node *corev1.Node) ([]*corev1.Pod, error) {
	var pods = make([]*corev1.Pod, 0)

	fromCache, err := h.podCache.GetByIndex(indexeres.PodByNodeNameIndex, node.Name)
	if err != nil {
		return pods, err
	}

	for _, pod := range fromCache {
		// kubectl uses this cache to filter out the pods
		if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
			continue
		}
		pods = append(pods, pod)
	}
	return pods, nil
}

func getPodResourceAnnotations(pods []*corev1.Pod) (string, string, error) {
	requests, limits := aggregateRequestAndLimitsForNode(pods)
	requestsBytes, err := json.Marshal(requests)
	if err != nil {
		return "", "", err
	}

	limitsBytes, err := json.Marshal(limits)
	return string(requestsBytes), string(limitsBytes), err
}

func aggregateRequestAndLimitsForNode(pods []*corev1.Pod) (map[corev1.ResourceName]resource.Quantity, map[corev1.ResourceName]resource.Quantity) {
	requests, limits := map[corev1.ResourceName]resource.Quantity{}, map[corev1.ResourceName]resource.Quantity{}
	for _, pod := range pods {
		podRequests, podLimits := getPodData(pod)
		addMap(podRequests, requests)
		addMap(podLimits, limits)
	}
	if pods != nil {
		requests[corev1.ResourcePods] = *resource.NewQuantity(int64(len(pods)), resource.DecimalSI)
	}
	return requests, limits
}

func getPodData(pod *corev1.Pod) (map[corev1.ResourceName]resource.Quantity, map[corev1.ResourceName]resource.Quantity) {
	requests, limits := map[corev1.ResourceName]resource.Quantity{}, map[corev1.ResourceName]resource.Quantity{}
	for _, container := range pod.Spec.Containers {
		addMap(container.Resources.Requests, requests)
		addMap(container.Resources.Limits, limits)
	}

	for _, container := range pod.Spec.InitContainers {
		addMapForInit(container.Resources.Requests, requests)
		addMapForInit(container.Resources.Limits, limits)
	}
	return requests, limits
}

func addMap(data1 map[corev1.ResourceName]resource.Quantity, data2 map[corev1.ResourceName]resource.Quantity) {
	for name, quantity := range data1 {
		if value, ok := data2[name]; !ok {
			data2[name] = quantity.DeepCopy()
		} else {
			value.Add(quantity)
			data2[name] = value
		}
	}
}

func addMapForInit(data1 map[corev1.ResourceName]resource.Quantity, data2 map[corev1.ResourceName]resource.Quantity) {
	for name, quantity := range data1 {
		value, ok := data2[name]
		if !ok {
			data2[name] = quantity.DeepCopy()
			continue
		}
		if quantity.Cmp(value) > 0 {
			data2[name] = quantity.DeepCopy()
		}
	}
}

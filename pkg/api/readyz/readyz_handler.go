package readyz

import (
	"time"

	"github.com/harvester/go-common/common"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
	longhornTypes "github.com/longhorn/longhorn-manager/types"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

type ReadyzHandler struct {
	podCache ctlcorev1.PodCache
	rkeCache generic.CacheInterface[*rkev1.RKEControlPlane]
}

func NewReadyzHandler(podCache ctlcorev1.PodCache, rkeCache generic.CacheInterface[*rkev1.RKEControlPlane]) *ReadyzHandler {
	return &ReadyzHandler{
		podCache: podCache,
		rkeCache: rkeCache,
	}
}

func (h *ReadyzHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	ready, msg := h.clusterReady()
	timestamp := time.Now().UTC().Format(time.RFC3339)

	if !ready {
		logrus.Debugf("Cluster not ready: %s", msg)
		ctx.SetStatusOK()
		return map[string]any{
			"ready":     false,
			"msg":       msg,
			"timestamp": timestamp,
		}, nil
	}

	ctx.SetStatusOK()
	return map[string]any{
		"ready":     true,
		"timestamp": timestamp,
	}, nil
}

func (h *ReadyzHandler) clusterReady() (bool, string) {
	if ready, msg := h.rkeReady(); !ready {
		return false, msg
	}

	if ready, msg := h.longhornReady(); !ready {
		return false, msg
	}

	if ready, msg := h.kubevirtReady(); !ready {
		return false, msg
	}

	return true, ""
}

func (h *ReadyzHandler) rkeReady() (bool, string) {
	rkeControlPlane, err := h.rkeCache.Get(
		util.FleetLocalNamespaceName,
		util.LocalClusterName)
	if err != nil {
		logrus.Debugf("rkeControlPlane not found: %s", err.Error())
		return false, "rkeControlPlane not found"
	}

	for _, cond := range rkeControlPlane.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == corev1.ConditionTrue {
			return true, ""
		}
	}
	return false, "rkeControlPlane is not ready"
}

func (h *ReadyzHandler) longhornReady() (bool, string) {
	longhornManagerSelector := labels.SelectorFromSet(labels.Set(longhornTypes.GetManagerLabels()))
	longhornPods, err := h.podCache.List(common.LonghornSystemNamespaceName, longhornManagerSelector)
	if err != nil {
		logrus.Debugf("failed to check longhorn-manager pods: %s", err.Error())
		return false, "failed to check longhorn-manager pods"
	}

	if !hasAtLeastOneReadyPod(longhornPods) {
		return false, "longhorn-manager pods not ready"
	}

	return true, ""
}

func (h *ReadyzHandler) kubevirtReady() (bool, string) {
	virtControllerSelector := labels.SelectorFromSet(labels.Set{kubevirtv1.AppLabel: "virt-controller"})
	virtPods, err := h.podCache.List(common.HarvesterSystemNamespaceName, virtControllerSelector)
	if err != nil {
		logrus.Debugf("failed to check virt-controller pods: %s", err.Error())
		return false, "failed to check virt-controller pods"
	}

	if !hasAtLeastOneReadyPod(virtPods) {
		return false, "virt-controller pods not ready"
	}

	return true, ""
}

func hasAtLeastOneReadyPod(pods []*corev1.Pod) bool {
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning && isPodReadyConditionTrue(pod) {
			return true
		}
	}
	return false
}

func isPodReadyConditionTrue(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

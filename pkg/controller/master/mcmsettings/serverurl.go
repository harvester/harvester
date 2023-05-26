package mcmsettings

import (
	"fmt"

	ranchersettings "github.com/rancher/rancher/pkg/settings"
	corev1 "k8s.io/api/core/v1"
)

const (
	PatchedByHarvesterKey   = "harvesterhci.io/patched-by-controller"
	PatchedByHarvesterValue = "true"
)

// updateServerURL will reconcile ingress-expose service in kube-system namespace and set the
// external IP to rancher server-url setting
func (h *mcmSettingsHandler) updateServerURL(key string, svc *corev1.Service) (*corev1.Service, error) {
	if svc == nil || svc.DeletionTimestamp != nil {
		return svc, nil
	}

	if key != vipService {
		return svc, nil
	}

	if len(svc.Status.LoadBalancer.Ingress) != 1 {
		return svc, fmt.Errorf("expected to find 1 load balancer ip for svc %s but got %v", key, svc.Spec.ExternalIPs)
	}

	serverURLSetting, err := h.rancherSettingsCache.Get(ranchersettings.ServerURL.Name)
	if err != nil {
		return svc, fmt.Errorf("error querying rancher server-url setting: %v", err)
	}

	if val, ok := serverURLSetting.Annotations[PatchedByHarvesterKey]; ok && val == PatchedByHarvesterValue {
		return svc, nil
	}

	serverURLSettingCopy := serverURLSetting.DeepCopy()
	serverURLSettingCopy.Value = fmt.Sprintf("https://%s", svc.Status.LoadBalancer.Ingress[0].IP)
	if serverURLSettingCopy.Annotations == nil {
		serverURLSettingCopy.Annotations = make(map[string]string)
	}

	serverURLSettingCopy.Annotations[PatchedByHarvesterKey] = PatchedByHarvesterValue
	if _, err := h.rancherSettingsClient.Update(serverURLSettingCopy); err != nil {
		return svc, err
	}

	return svc, nil
}

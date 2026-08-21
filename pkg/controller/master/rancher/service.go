package rancher

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/harvester/harvester/pkg/util"
)

// registerExposeService help to create ingress-expose svc in the kube-system namespace,
// by default it is nodePort, if the VIP is enabled it will be set to LoadBalancer type service.
func (h *Handler) registerExposeService() error {
	// verify if traefik exists
	traefikExists, err := h.doesTraefikExist()
	if err != nil {
		return err
	}

	// if traefik does not exist yet, then we continue working
	// as normal operation and using ingress
	if !traefikExists {
		_, err := h.Services.Get(util.KubeSystemNamespace, ingressExposeServiceName, v1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if apierrors.IsNotFound(err) {
			return h.createIngressExposeService()
		}
		return nil
	}

	// clean up old nginx service as it is no longer needed
	if err := h.cleanupIngressExpose(); err != nil {
		return fmt.Errorf("error cleaning up ingress expose service: %w", err)
	}
	// traefik exists post rke2 upgrade
	// and we can just traefik service annotations
	return h.patchTraefikServiceAnnotations()

}

func (h *Handler) createIngressExposeService() error {
	vip, err := h.getVipConfig()
	if err != nil {
		return err
	}

	svc := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      ingressExposeServiceName,
			Namespace: util.KubeSystemNamespace,
			Annotations: map[string]string{
				keyKubevipIgnoreServiceSecurity: trueStr,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				appLabelName: util.Rke2IngressNginxAppName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "https-internal",
					Port:       443,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(443),
				},
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}

	// set vip loadBalancer type and ip
	enabled, err := strconv.ParseBool(vip.Enabled)
	if err != nil {
		return err
	}
	if enabled && vip.ServiceType == corev1.ServiceTypeLoadBalancer {
		svc.Spec.Type = vip.ServiceType
		// After kube-vip v0.5.2, it uses annotation kube-vip.io/loadbalancerIPs to set the loadBalancerIP
		if strings.ToLower(vip.Mode) == vipDHCPMode {
			svc.Annotations[keyKubevipRequestIP] = vip.IP
			svc.Annotations[keyKubevipHwaddr] = vip.HwAddress
			svc.Annotations[keyKubevipLoadBalancerIPs] = vipDHCPLoadBalancerIP
		} else {
			svc.Annotations[keyKubevipLoadBalancerIPs] = vip.IP
		}
	}

	if _, err := h.Services.Create(svc); err != nil {
		return err
	}

	return nil
}

// cleanUpLegacyExposeService removes rancher-expose service in cattle-system
func (h *Handler) cleanUpLegacyExposeService() error {
	if err := h.Services.Delete(util.CattleSystemNamespaceName, rancherExposeServiceName, &v1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (h *Handler) getVipConfig() (*VIPConfig, error) {
	vipConfig := &VIPConfig{}
	conf, err := h.Configmaps.Get(h.Namespace, VipConfigmapName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if err := mapstructure.Decode(conf.Data, vipConfig); err != nil {
		return nil, err
	}
	return vipConfig, nil
}

func (h *Handler) rolloutKubevipDaemonSet(reason string) error {
	now := time.Now().Format(time.RFC3339)
	stampedReason := fmt.Sprintf("[%s] %s", now, reason)

	patchPayload := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						// Generic standard key "kubectl.kubernetes.io/restartedAt" for kubectl compatibility
						util.AnnotationKubectlRestartedAt: now,
						// Harvester controller specific tracking
						util.AnnotationHarvesterRestartedBy:   controllerRancherName,
						util.AnnotationHarvesterRestartedAt:   now,
						util.AnnotationHarvesterRestartReason: stampedReason,
					},
				},
			},
		},
	}

	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal restart patch payload: %w", err)
	}

	_, err = h.DaemonSetClient.Patch(
		util.HarvesterSystemNamespaceName,
		kubeVipDaemonSetName,
		types.StrategicMergePatchType,
		patchBytes,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Note: If the kube-vip DaemonSet is missing/deleted, gracefully skip the rollout
			// restart to avoid throwing errors that cause the Harvester controller to crash loop.
			logrus.Warnf("DaemonSet %s not found, skipping rollout restart for: %s", kubeVipDaemonSetName, stampedReason)
			return nil
		}
		return fmt.Errorf("failed to rollout restart daemonset %s: %w", kubeVipDaemonSetName, err)
	}
	return nil
}

// checks if rke2-traefik service already exists
// which means underlying rke2 has been swapped out to using traefik as
// ingress controller
func (h *Handler) doesTraefikExist() (bool, error) {
	_, err := h.Services.Get(util.KubeSystemNamespace, traefikServiceName, v1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// patchTraefikServiceAnnotations will update the traefik service with kubevip annotations
// if needed
func (h *Handler) patchTraefikServiceAnnotations() error {
	svc, err := h.Services.Get(util.KubeSystemNamespace, traefikServiceName, v1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("service %s/%s not found, skipping annotation patch", util.KubeSystemNamespace, traefikServiceName)
			return nil
		}
		return fmt.Errorf("error fetching traefik service while attempting to update annotations: %w", err)
	}

	vip, err := h.getVipConfig()
	if err != nil {
		return fmt.Errorf("error fetching vip configuration while attempt to update traefik service annotations: %w", err)
	}

	// set vip loadBalancer type and ip
	enabled, err := strconv.ParseBool(vip.Enabled)
	if err != nil {
		return err
	}

	svcCopy := svc.DeepCopy()

	if enabled && vip.ServiceType == corev1.ServiceTypeLoadBalancer {
		svcCopy.Spec.Type = vip.ServiceType
		if svcCopy.Annotations == nil {
			svcCopy.Annotations = make(map[string]string)
		}
		svcCopy.Annotations[keyKubevipIgnoreServiceSecurity] = trueStr
		// After kube-vip v0.5.2, it uses annotation kube-vip.io/loadbalancerIPs to set the loadBalancerIP
		if strings.ToLower(vip.Mode) == vipDHCPMode {
			svcCopy.Annotations[keyKubevipRequestIP] = vip.IP
			svcCopy.Annotations[keyKubevipHwaddr] = vip.HwAddress
			svcCopy.Annotations[keyKubevipLoadBalancerIPs] = vipDHCPLoadBalancerIP
		} else {
			svcCopy.Annotations[keyKubevipLoadBalancerIPs] = vip.IP
		}
	}
	if !reflect.DeepEqual(svc, svcCopy) {
		reason := fmt.Sprintf("Service %s (old type %s) is updated to align with vip config and DaemonSet %s is rolled out", traefikServiceName, svc.Spec.Type, kubeVipDaemonSetName)
		logrus.Infof("%s", reason)
		if _, err = h.Services.Update(svcCopy); err != nil {
			return fmt.Errorf("error updating %s svc: %w", svcCopy.Name, err)
		}

		// svc got updated, rollout kubevip to ensure changes take effect
		// Note: If rollouttKubevipDaemonSet fails here, it won't re-execute on subsequent reconciliations
		// because reflect.DeepEqual(svc, svcCopy) will evaluate to true since the Service update already succeeded.
		// This will be optimized later.
		return h.rolloutKubevipDaemonSet(reason)
	}

	return err
}

// remove nginx ingress expose service
func (h *Handler) cleanupIngressExpose() error {
	err := h.Services.Delete(util.KubeSystemNamespace, ingressExposeServiceName, &v1.DeleteOptions{})
	if !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

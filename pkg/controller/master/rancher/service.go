package rancher

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/labels"
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

func (h *Handler) restartKubevipPods() error {
	pods, err := h.podCache.List(util.HarvesterSystemNamespaceName, labels.Set(map[string]string{
		"app.kubernetes.io/name": "kube-vip",
	}).AsSelector())
	if err != nil {
		return err
	}

	for i := range pods {
		if err := h.podClient.Delete(util.HarvesterSystemNamespaceName, pods[i].Name, &v1.DeleteOptions{}); err != nil {
			return err
		}
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
		svc.Spec.Type = vip.ServiceType
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations[keyKubevipIgnoreServiceSecurity] = trueStr
		// After kube-vip v0.5.2, it uses annotation kube-vip.io/loadbalancerIPs to set the loadBalancerIP
		if strings.ToLower(vip.Mode) == vipDHCPMode {
			svc.Annotations[keyKubevipRequestIP] = vip.IP
			svc.Annotations[keyKubevipHwaddr] = vip.HwAddress
			svc.Annotations[keyKubevipLoadBalancerIPs] = vipDHCPLoadBalancerIP
		} else {
			svc.Annotations[keyKubevipLoadBalancerIPs] = vip.IP
		}
	}
	if !reflect.DeepEqual(svc, svcCopy) {
		if _, err = h.Services.Update(svc); err != nil {
			return fmt.Errorf("error updating %s svc: %w", svc.Name, err)
		}

		// svc got updated, restart kubevip to ensure changes take effect
		return h.restartKubevipPods()
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

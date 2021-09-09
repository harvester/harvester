package rancher

import (
	"context"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerRancherName     = "harvester-rancher-controller"
	cattleSystemNamespaceName = "cattle-system"
	rancherExposeServiceName  = "rancher-expose"
	rancherAppLabelName       = "app"

	vipConfigmapName      = "vip"
	vipDHCPMode           = "dhcp"
	vipDHCPLoadBalancerIP = "0.0.0.0"
)

type Handler struct {
	RancherSettings rancherv3.SettingClient
	Services        ctlcorev1.ServiceClient
	Configmaps      ctlcorev1.ConfigMapClient
	Namespace       string
}

type VIPConfig struct {
	Enabled        string             `json:"enabled,omitempty"`
	ServiceType    corev1.ServiceType `json:"serviceType,omitempty"`
	IP             string             `json:"ip,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	HwAddress      string             `json:"hwAddress,omitempty"`
	LoadBalancerIP string             `json:"loadBalancerIP,omitempty"`
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if options.RancherEmbedded {
		rancherSettings := management.RancherManagementFactory.Management().V3().Setting()
		services := management.CoreFactory.Core().V1().Service()
		configmaps := management.CoreFactory.Core().V1().ConfigMap()
		h := Handler{
			RancherSettings: rancherSettings,
			Services:        services,
			Configmaps:      configmaps,
			Namespace:       options.Namespace,
		}

		rancherSettings.OnChange(ctx, controllerRancherName, h.RancherSettingOnChange)
		return h.registerRancherExposeService()
	}
	return nil
}

// registerRancherExposeService help to create rancher-expose svc in the cattle-system namespace,
// by default it is nodePort, if the VIP is enabled it will be set to LoadBalancer type service.
func (h *Handler) registerRancherExposeService() error {
	_, err := h.Services.Get(cattleSystemNamespaceName, rancherExposeServiceName, v1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if apierrors.IsNotFound(err) {
		vip, err := h.getVipConfig()
		if err != nil {
			return err
		}

		svc := &corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      rancherExposeServiceName,
				Namespace: cattleSystemNamespaceName,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeNodePort,
				Selector: map[string]string{
					rancherAppLabelName: "rancher",
				},
				Ports: []corev1.ServicePort{
					{
						Name:       "https-internal",
						NodePort:   30443,
						Port:       443,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(444),
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
			svc.Spec.LoadBalancerIP = vip.LoadBalancerIP
			if strings.ToLower(vip.Mode) == vipDHCPMode {
				svc.Annotations = map[string]string{
					"kube-vip.io/requestedIP": vip.IP,
					"kube-vip.io/hwaddr":      vip.HwAddress,
				}
				svc.Spec.LoadBalancerIP = vipDHCPLoadBalancerIP
			}
		}

		if _, err := h.Services.Create(svc); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) getVipConfig() (*VIPConfig, error) {
	vipConfig := &VIPConfig{}
	conf, err := h.Configmaps.Get(h.Namespace, vipConfigmapName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if err := mapstructure.Decode(conf.Data, vipConfig); err != nil {
		return nil, err
	}
	return vipConfig, nil
}

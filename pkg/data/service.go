package data

import (
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
)

const (
	ingressExposeServiceName = "ingress-expose"
	vipConfigMapName         = "vip"

	appLabelName                               = "app.kubernetes.io/name"
	kubevipIgnoreServiceSecurityAnnotationName = "kube-vip.io/ignore-service-security"
	kubevipHwaddrAnnotationName                = "kube-vip.io/hwaddr"
	kubevipRequestIPAnnotationName             = "kube-vip.io/requestedIP"
	kubevipLoadBalancerIPsAnnotationName       = "kube-vip.io/loadbalancerIPs"

	vipDHCPMode           = "dhcp"
	vipDHCPLoadBalancerIP = "0.0.0.0"
)

type VIPConfig struct {
	Enabled        string             `json:"enabled,omitempty"`
	ServiceType    corev1.ServiceType `json:"serviceType,omitempty"`
	IP             string             `json:"ip,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	HwAddress      string             `json:"hwAddress,omitempty"`
	LoadBalancerIP string             `json:"loadBalancerIP,omitempty"`
}

func createServices(mgmt *config.Management) error {
	services := mgmt.CoreFactory.Core().V1().Service()
	configMaps := mgmt.CoreFactory.Core().V1().ConfigMap()

	_, err := services.Get(util.KubeSystemNamespace, ingressExposeServiceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return createIngressExposeService(services, configMaps)
		}
		return err
	}
	return nil
}

func createIngressExposeService(services v1.ServiceClient, configMaps v1.ConfigMapClient) error {
	vip, err := getVipConfig(configMaps)
	if err != nil {
		return err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressExposeServiceName,
			Namespace: util.KubeSystemNamespace,
			Annotations: map[string]string{
				kubevipIgnoreServiceSecurityAnnotationName: "true",
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
			svc.Annotations[kubevipRequestIPAnnotationName] = vip.IP
			svc.Annotations[kubevipHwaddrAnnotationName] = vip.HwAddress
			svc.Annotations[kubevipLoadBalancerIPsAnnotationName] = vipDHCPLoadBalancerIP
		} else {
			svc.Annotations[kubevipLoadBalancerIPsAnnotationName] = vip.IP
		}
	}

	if _, err := services.Create(svc); err != nil {
		return err
	}
	return nil
}

func getVipConfig(configMaps v1.ConfigMapClient) (*VIPConfig, error) {
	cm, err := configMaps.Get(util.HarvesterSystemNamespaceName, vipConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	vipConfig := &VIPConfig{}
	if err := mapstructure.Decode(cm.Data, vipConfig); err != nil {
		return nil, err
	}
	return vipConfig, nil
}

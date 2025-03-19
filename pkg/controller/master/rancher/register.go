package rancher

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	networkingv1 "github.com/harvester/harvester/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	appLabelName             = "app.kubernetes.io/name"
	controllerRancherName    = "harvester-rancher-controller"
	controllerNamespaceName  = "harvester-namespace-controller"
	caCertsSetting           = "cacerts"
	defaultAdminLabelKey     = "authz.management.cattle.io/bootstrapping"
	defaultAdminLabelValue   = "admin-user"
	internalCACertsSetting   = "internal-cacerts"
	rancherExposeServiceName = "rancher-expose"
	ingressExposeServiceName = "ingress-expose"
	systemNamespacesSetting  = "system-namespaces"
	tlsCNPrefix              = "listener.cattle.io/cn-"

	keyKubevipRequestIP             = "kube-vip.io/requestedIP"
	keyKubevipHwaddr                = "kube-vip.io/hwaddr"
	keyKubevipIgnoreServiceSecurity = "kube-vip.io/ignore-service-security"
	keyKubevipLoadBalancerIPs       = "kube-vip.io/loadbalancerIPs"

	VipConfigmapName                  = "vip"
	vipDHCPMode                       = "dhcp"
	vipDHCPLoadBalancerIP             = "0.0.0.0"
	trueStr                           = "true"
	controllerCAPIDeployment          = "harvester-capi-controller"
	capiControllerDeploymentName      = "capi-controller-manager"
	capiControllerDeploymentNamespace = "cattle-provisioning-capi-system"
)

type Handler struct {
	RancherSettings          rancherv3.SettingClient
	RancherSettingCache      rancherv3.SettingCache
	RancherSettingController rancherv3.SettingController
	RancherUserCache         rancherv3.UserCache
	ingresses                networkingv1.IngressClient
	Services                 ctlcorev1.ServiceClient
	Configmaps               ctlcorev1.ConfigMapClient
	Secrets                  ctlcorev1.SecretClient
	SecretCache              ctlcorev1.SecretCache
	nodeController           ctlcorev1.NodeController
	podCache                 ctlcorev1.PodCache
	podClient                ctlcorev1.PodClient
	Deployments              ctlappsv1.DeploymentClient
	Namespace                string
	RancherTokenController   rancherv3.TokenController
	SettingCache             v1beta1.SettingCache
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
		rancherUsers := management.RancherManagementFactory.Management().V3().User()
		rancherTokens := management.RancherManagementFactory.Management().V3().Token()
		ingresses := management.NetworkingFactory.Networking().V1().Ingress()
		secrets := management.CoreFactory.Core().V1().Secret()
		services := management.CoreFactory.Core().V1().Service()
		configmaps := management.CoreFactory.Core().V1().ConfigMap()
		nodes := management.CoreFactory.Core().V1().Node()
		pods := management.CoreFactory.Core().V1().Pod()
		namespaces := management.CoreFactory.Core().V1().Namespace()
		deployments := management.AppsFactory.Apps().V1().Deployment()
		settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
		h := Handler{
			RancherSettings:          rancherSettings,
			RancherSettingController: rancherSettings,
			RancherSettingCache:      rancherSettings.Cache(),
			RancherUserCache:         rancherUsers.Cache(),
			RancherTokenController:   rancherTokens,
			ingresses:                ingresses,
			Services:                 services,
			Configmaps:               configmaps,
			Secrets:                  secrets,
			SecretCache:              secrets.Cache(),
			nodeController:           nodes,
			podCache:                 pods.Cache(),
			podClient:                pods,
			Namespace:                options.Namespace,
			Deployments:              deployments,
			SettingCache:             settings.Cache(),
		}
		nodes.OnChange(ctx, controllerRancherName, h.PodResourcesOnChanged)
		rancherSettings.OnChange(ctx, controllerRancherName, h.RancherSettingOnChange)
		secrets.OnChange(ctx, controllerRancherName, h.TLSSecretOnChange)
		deployments.OnChange(ctx, controllerCAPIDeployment, h.PatchCAPIDeployment)
		rancherTokens.OnChange(ctx, controllerRancherName, h.RancherTokenOnChange)
		namespaces.OnRemove(ctx, controllerNamespaceName, h.onNamespaceRemoved)

		if err := h.registerExposeService(); err != nil {
			return err
		}
		if err := h.cleanUpLegacyExposeService(); err != nil {
			return err
		}
	}

	return nil
}

// registerExposeService help to create ingress-expose svc in the kube-system namespace,
// by default it is nodePort, if the VIP is enabled it will be set to LoadBalancer type service.
func (h *Handler) registerExposeService() error {
	svc, err := h.Services.Get(util.KubeSystemNamespace, ingressExposeServiceName, v1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if apierrors.IsNotFound(err) {
		return h.createIngressExposeService()
	}
	// Update annotations of the ingress-expose service created before Harvester v1.2.0
	if svc.Annotations == nil || svc.Annotations[keyKubevipIgnoreServiceSecurity] != trueStr {
		return h.updateIngressExposeService(svc)
	}

	return nil
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

func (h *Handler) updateIngressExposeService(svc *corev1.Service) error {
	svcCopy := svc.DeepCopy()
	if svc.Annotations == nil {
		svcCopy.Annotations = make(map[string]string)
	}
	svcCopy.Annotations[keyKubevipIgnoreServiceSecurity] = trueStr
	if _, ok := svcCopy.Annotations[keyKubevipLoadBalancerIPs]; !ok {
		svcCopy.Annotations[keyKubevipLoadBalancerIPs] = svc.Spec.LoadBalancerIP
	}
	if _, err := h.Services.Update(svcCopy); err != nil {
		return err
	}
	if err := h.restartKubevipPods(); err != nil {
		return fmt.Errorf("failed to restart kube-vip pods: %w", err)
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

// RancherTokenOnChange updates the expiresAt field of the token.
// Although we have embedded rancher, we don't start the rancher's token controller.
// So, we should have our own handler to update the token.
func (h *Handler) RancherTokenOnChange(_ string, token *v3.Token) (*v3.Token, error) {
	if token == nil || token.DeletionTimestamp != nil {
		return nil, nil
	}

	if token.TTLMillis != 0 && token.ExpiresAt == "" {
		//compute and save expiresAt
		newToken := token.DeepCopy()
		var err error

		created := newToken.ObjectMeta.CreationTimestamp.Time
		ttlDuration := time.Duration(newToken.TTLMillis) * time.Millisecond
		expiresAtTime := created.Add(ttlDuration)
		newToken.ExpiresAt = expiresAtTime.UTC().Format(time.RFC3339)

		if newToken, err = h.RancherTokenController.Update(newToken); err != nil {
			return token, err
		}

		token = newToken
	}

	return token, nil
}

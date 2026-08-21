package rancher

import (
	"context"
	"fmt"
	"time"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	networkingv1 "github.com/harvester/harvester/pkg/generated/controllers/networking.k8s.io/v1"
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
	traefikServiceName       = "rke2-traefik"
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
	capiControllerDeploymentNamespace = "cattle-capi-system"
	daemonSetsController              = "daemonset-controller"
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
	NamespaceCache           ctlcorev1.NamespaceCache
	NamespaceController      ctlcorev1.NamespaceController
	NamespaceClient          ctlcorev1.NamespaceClient
	SettingClient            v1beta1.SettingClient
	DynamicClient            dynamic.Interface
	ctx                      context.Context
	RestConfig               *rest.Config
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
		daemonSets := management.AppsFactory.Apps().V1().DaemonSet()
		dynamicClient, err := dynamic.NewForConfig(management.RestConfig)

		if err != nil {
			return fmt.Errorf("error generating dynamic client during rancher handler registration: %v", err)
		}
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
			NamespaceCache:           namespaces.Cache(),
			NamespaceClient:          namespaces,
			NamespaceController:      namespaces,
			SettingClient:            settings,
			DynamicClient:            dynamicClient,
			ctx:                      ctx,
			RestConfig:               management.RestConfig,
		}

		nodes.OnChange(ctx, controllerRancherName, h.PodResourcesOnChanged)
		rancherSettings.OnChange(ctx, controllerRancherName, h.RancherSettingOnChange)
		secrets.OnChange(ctx, controllerRancherName, h.TLSSecretOnChange)
		deployments.OnChange(ctx, controllerCAPIDeployment, h.PatchCAPIDeployment)
		rancherTokens.OnChange(ctx, controllerRancherName, h.RancherTokenOnChange)
		namespaces.OnRemove(ctx, controllerNamespaceName, h.onNamespaceRemoved)
		namespaces.OnChange(ctx, controllerNamespaceName, h.onNamespaceChanged)
		daemonSets.OnChange(ctx, daemonSetsController, h.reconcileIngressResources)

		if err := h.registerExposeService(); err != nil {
			return err
		}
		if err := h.cleanUpLegacyExposeService(); err != nil {
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

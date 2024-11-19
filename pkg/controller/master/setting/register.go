package setting

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/config"
	harvSettings "github.com/harvester/harvester/pkg/settings"
)

const (
	controllerName = "harvester-setting-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	clusters := management.ProvisioningFactory.Provisioning().V1().Cluster()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	configmaps := management.CoreFactory.Core().V1().ConfigMap()
	services := management.CoreFactory.Core().V1().Service()
	lhs := management.LonghornFactory.Longhorn().V1beta2().Setting()
	ingresses := management.NetworkingFactory.Networking().V1().Ingress()
	helmChartConfigs := management.HelmFactory.Helm().V1().HelmChartConfig()
	nodeConfigs := management.NodeConfigFactory.Node().V1beta1().NodeConfig()
	node := management.CoreFactory.Core().V1().Node()
	rkeControlPlane := management.RKEFactory.Rke().V1().RKEControlPlane()
	kubevirt := management.VirtFactory.Kubevirt().V1().KubeVirt()
	bundles := management.FleetFactory.Fleet().V1alpha1().Bundle()

	restClientGetter := cli.New().RESTClientGetter()
	kubeClient := kube.New(restClientGetter)
	var kubeInterface kubernetes.Interface
	kubeInterface = management.ClientSet
	driverSecret := driver.NewSecrets(kubeInterface.CoreV1().Secrets(""))
	store := storage.Init(driverSecret)

	controller := &Handler{
		namespace:            options.Namespace,
		apply:                management.Apply,
		settings:             settings,
		settingCache:         settings.Cache(),
		settingController:    settings,
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		clusters:             clusters,
		clusterCache:         clusters.Cache(),
		deployments:          deployments,
		deploymentCache:      deployments.Cache(),
		ingresses:            ingresses,
		ingressCache:         ingresses.Cache(),
		longhornSettings:     lhs,
		longhornSettingCache: lhs.Cache(),
		configmaps:           configmaps,
		configmapCache:       configmaps.Cache(),
		serviceCache:         services.Cache(),
		helmChartConfigs:     helmChartConfigs,
		helmChartConfigCache: helmChartConfigs.Cache(),
		nodeConfigs:          nodeConfigs,
		nodeConfigsCache:     nodeConfigs.Cache(),
		nodeClient:           node,
		nodeCache:            node.Cache(),
		rkeControlPlaneCache: rkeControlPlane.Cache(),
		kubeVirtConfig:       kubevirt,
		kubeVirtConfigCache:  kubevirt.Cache(),
		bundleClient:         bundles,
		bundleCache:          bundles.Cache(),

		httpClient: http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},

		helmConfiguration: action.Configuration{
			RESTClientGetter: restClientGetter,
			Releases:         store,
			KubeClient:       kubeClient,
		},
	}

	syncers = map[string]syncerFunc{
		"additional-ca":                    controller.syncAdditionalTrustedCAs,
		"cluster-registration-url":         controller.registerCluster,
		"http-proxy":                       controller.syncHTTPProxy,
		"log-level":                        controller.setLogLevel,
		"overcommit-config":                controller.syncOvercommitConfig,
		"vip-pools":                        controller.syncVipPoolsConfig,
		"auto-disk-provision-paths":        controller.syncNDMAutoProvisionPaths,
		"ssl-certificates":                 controller.syncSSLCertificate,
		"ssl-parameters":                   controller.syncSSLParameters,
		harvSettings.NTPServersSettingName: controller.syncNodeConfig,
		harvSettings.LonghornV2DataEngineSettingName:        controller.syncNodeConfig,
		"auto-rotate-rke2-certs":                            controller.syncAutoRotateRKE2Certs,
		harvSettings.AdditionalGuestMemoryOverheadRatioName: controller.syncAdditionalGuestMemoryOverheadRatio,
		harvSettings.SupportBundleImageName:                 controller.syncSupportBundleImage,
		harvSettings.GeneralJobImageName:                    controller.syncGeneralJobImage,
		// for "backup-target" syncer, please check harvester-backup-target-controller
		// for "storage-network" syncer, please check harvester-storage-network-controller
	}

	settings.OnChange(ctx, controllerName, controller.settingOnChanged)
	node.OnChange(ctx, controllerName, controller.nodeOnChanged)
	return nil
}

package mcmsettings

import (
	"context"

	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlrancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"

	"github.com/harvester/harvester/pkg/config"
)

const (
	clusterRepoHandler     = "cluster-repo-handler"
	watchFleetController   = "watch-fleet-controller-cm"
	rancherSettingsHandler = "harvester-rancher-settings-handler"
)

type mcmSettingsHandler struct {
	rancherSettingsClient ctlrancherv3.SettingClient
	rancherSettingsCache  ctlrancherv3.SettingCache
	clusterRepoClient     ctlcatalogv1.ClusterRepoClient
	configMapCache        ctlcorev1.ConfigMapCache
	configMapClient       ctlcorev1.ConfigMapClient
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	rancherSettingController := management.RancherManagementFactory.Management().V3().Setting()
	clusterRepoController := management.CatalogFactory.Catalog().V1().ClusterRepo()
	configMapController := management.CoreFactory.Core().V1().ConfigMap()
	h := &mcmSettingsHandler{
		rancherSettingsClient: rancherSettingController,
		rancherSettingsCache:  rancherSettingController.Cache(),
		clusterRepoClient:     clusterRepoController,
		configMapCache:        configMapController.Cache(),
		configMapClient:       configMapController,
	}

	clusterRepoController.OnChange(ctx, clusterRepoHandler, h.patchClusterRepos)
	rancherSettingController.OnChange(ctx, rancherSettingsHandler, h.watchInternalEndpointSettings)
	relatedresource.WatchClusterScoped(ctx, watchFleetController, h.watchFleetControllerConfigMap, rancherSettingController, configMapController)
	return nil
}

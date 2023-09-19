package mcmsettings

import (
	"context"

	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlrancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"

	"github.com/harvester/harvester/pkg/config"
)

const (
	clusterRepoHandler = "cluster-repo-handler"
)

type mcmSettingsHandler struct {
	rancherSettingsClient ctlrancherv3.SettingClient
	rancherSettingsCache  ctlrancherv3.SettingCache
	clusterRepoClient     ctlcatalogv1.ClusterRepoClient
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	rancherSettingController := management.RancherManagementFactory.Management().V3().Setting()
	clusterRepoController := management.CatalogFactory.Catalog().V1().ClusterRepo()

	h := &mcmSettingsHandler{
		rancherSettingsClient: rancherSettingController,
		rancherSettingsCache:  rancherSettingController.Cache(),
		clusterRepoClient:     clusterRepoController,
	}

	clusterRepoController.OnChange(ctx, clusterRepoHandler, h.patchClusterRepos)
	return nil
}

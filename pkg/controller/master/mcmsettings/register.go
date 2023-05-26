package mcmsettings

import (
	"context"

	ctlcatalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlrancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"

	"github.com/harvester/harvester/pkg/config"
)

const (
	serverURLHandler   = "server-url-handler"
	vipService         = "kube-system/ingress-expose"
	clusterRepoHandler = "cluster-repo-handler"
)

type mcmSettingsHandler struct {
	svcCache              ctlcorev1.ServiceCache
	rancherSettingsClient ctlrancherv3.SettingClient
	rancherSettingsCache  ctlrancherv3.SettingCache
	clusterRepoClient     ctlcatalogv1.ClusterRepoClient
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	svcController := management.CoreFactory.Core().V1().Service()
	rancherSettingController := management.RancherManagementFactory.Management().V3().Setting()
	clusterRepoController := management.CatalogFactory.Catalog().V1().ClusterRepo()

	h := &mcmSettingsHandler{
		svcCache:              svcController.Cache(),
		rancherSettingsClient: rancherSettingController,
		rancherSettingsCache:  rancherSettingController.Cache(),
		clusterRepoClient:     clusterRepoController,
	}

	svcController.OnChange(ctx, serverURLHandler, h.updateServerURL)
	clusterRepoController.OnChange(ctx, clusterRepoHandler, h.patchClusterRepos)
	return nil
}

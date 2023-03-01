package upgradelog

import (
	"net/http"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	upgradeLogHandler := Handler{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		jobClient:        scaled.BatchFactory.Batch().V1().Job(),
		podCache:         scaled.CoreFactory.Core().V1().Pod().Cache(),
		upgradeCache:     scaled.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache(),
		upgradeLogCache:  scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog().Cache(),
		upgradeLogClient: scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog(),
	}

	t := schema.Template{
		ID: "harvesterhci.io.upgradelog",
		Customize: func(s *types.APISchema) {
			s.ResourceActions = map[string]schemas.Action{
				generateArchiveAction: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				generateArchiveAction: upgradeLogHandler,
			}
			s.LinkHandlers = map[string]http.Handler{
				downloadArchiveLink: upgradeLogHandler,
			}
		},
		Formatter: Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}

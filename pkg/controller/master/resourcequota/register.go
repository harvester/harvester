package resourcequota

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	resourceQuotaControllerName = "resourceQuotaController"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	rqs := management.HarvesterCoreFactory.Core().V1().ResourceQuota()
	handler := &Handler{
		rqs:     rqs,
		rqCache: rqs.Cache(),
	}
	rqs.OnChange(ctx, resourceQuotaControllerName, handler.OnResourceQuotaChanged)
	return nil
}

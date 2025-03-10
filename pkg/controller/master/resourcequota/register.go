package resourcequota

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	resourceQuotaControllerName = "resourceQuotaController"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	rqs := management.HarvesterCoreFactory.Core().V1().ResourceQuota()
	nsCache := management.CoreFactory.Core().V1().Namespace().Cache()
	handler := &Handler{
		nsCache: nsCache,
		rqs:     rqs,
		rqCache: rqs.Cache(),
	}
	rqs.OnChange(ctx, resourceQuotaControllerName, handler.OnResourceQuotaChanged)
	return nil
}

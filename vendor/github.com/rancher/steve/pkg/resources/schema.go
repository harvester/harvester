package resources

import (
	"context"

	"github.com/rancher/apiserver/pkg/store/apiroot"
	"github.com/rancher/apiserver/pkg/subscribe"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/client"
	"github.com/rancher/steve/pkg/clustercache"
	"github.com/rancher/steve/pkg/resources/apigroups"
	"github.com/rancher/steve/pkg/resources/cluster"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/resources/counts"
	"github.com/rancher/steve/pkg/resources/formatters"
	"github.com/rancher/steve/pkg/resources/userpreferences"
	"github.com/rancher/steve/pkg/schema"
	steveschema "github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/steve/pkg/summarycache"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/discovery"
)

func DefaultSchemas(ctx context.Context, baseSchema *types.APISchemas, ccache clustercache.ClusterCache,
	cg proxy.ClientGetter, schemaFactory steveschema.Factory, serverVersion string) error {
	counts.Register(baseSchema, ccache)
	subscribe.Register(baseSchema, func(apiOp *types.APIRequest) *types.APISchemas {
		user, ok := request.UserFrom(apiOp.Context())
		if ok {
			schemas, err := schemaFactory.Schemas(user)
			if err == nil {
				return schemas
			}
		}
		return apiOp.Schemas
	}, serverVersion)
	apiroot.Register(baseSchema, []string{"v1"}, "proxy:/apis")
	cluster.Register(ctx, baseSchema, cg, schemaFactory)
	userpreferences.Register(baseSchema)
	return nil
}

func DefaultSchemaTemplates(cf *client.Factory,
	baseSchemas *types.APISchemas,
	summaryCache *summarycache.SummaryCache,
	lookup accesscontrol.AccessSetLookup,
	discovery discovery.DiscoveryInterface) []schema.Template {
	return []schema.Template{
		common.DefaultTemplate(cf, summaryCache, lookup),
		apigroups.Template(discovery),
		{
			ID:        "configmap",
			Formatter: formatters.DropHelmData,
		},
		{
			ID:        "secret",
			Formatter: formatters.DropHelmData,
		},
		{
			ID:        "pod",
			Formatter: formatters.Pod,
		},
		{
			ID: "management.cattle.io.cluster",
			Customize: func(apiSchema *types.APISchema) {
				cluster.AddApply(baseSchemas, apiSchema)
			},
		},
	}
}

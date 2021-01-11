package common

import (
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/steve/pkg/summarycache"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/rancher/wrangler/pkg/summary"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func DefaultTemplate(clientGetter proxy.ClientGetter,
	summaryCache *summarycache.SummaryCache,
	asl accesscontrol.AccessSetLookup) schema.Template {
	return schema.Template{
		Store:     proxy.NewProxyStore(clientGetter, summaryCache, asl),
		Formatter: formatter(summaryCache),
	}
}

func formatter(summarycache *summarycache.SummaryCache) types.Formatter {
	return func(request *types.APIRequest, resource *types.RawResource) {
		meta, err := meta.Accessor(resource.APIObject.Object)
		if err != nil {
			return
		}

		selfLink := meta.GetSelfLink()
		if selfLink == "" {
			return
		}

		u := request.URLBuilder.RelativeToRoot(selfLink)
		resource.Links["view"] = u

		if _, ok := resource.Links["update"]; !ok && slice.ContainsString(resource.Schema.CollectionMethods, "PUT") {
			resource.Links["update"] = u
		}

		if unstr, ok := resource.APIObject.Object.(*unstructured.Unstructured); ok {
			s, rel := summarycache.SummaryAndRelationship(unstr)
			data.PutValue(unstr.Object, map[string]interface{}{
				"name":          s.State,
				"error":         s.Error,
				"transitioning": s.Transitioning,
				"message":       strings.Join(s.Message, ":"),
			}, "metadata", "state")
			data.PutValue(unstr.Object, rel, "metadata", "relationships")

			summary.NormalizeConditions(unstr)
		}
	}
}

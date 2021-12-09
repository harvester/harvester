package common

import (
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/steve/pkg/summarycache"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/rancher/wrangler/pkg/summary"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	schema2 "k8s.io/apimachinery/pkg/runtime/schema"
)

func DefaultTemplate(clientGetter proxy.ClientGetter,
	summaryCache *summarycache.SummaryCache,
	asl accesscontrol.AccessSetLookup) schema.Template {
	return schema.Template{
		Store:     proxy.NewProxyStore(clientGetter, summaryCache, asl),
		Formatter: formatter(summaryCache),
	}
}

func selfLink(gvr schema2.GroupVersionResource, meta metav1.Object) (prefix string) {
	buf := &strings.Builder{}
	if gvr.Group == "" {
		buf.WriteString("/api/v1/")
	} else {
		buf.WriteString("/apis/")
		buf.WriteString(gvr.Group)
		buf.WriteString("/")
		buf.WriteString(gvr.Version)
		buf.WriteString("/")
	}
	if meta.GetNamespace() != "" {
		buf.WriteString("namespaces/")
		buf.WriteString(meta.GetNamespace())
		buf.WriteString("/")
	}
	buf.WriteString(gvr.Resource)
	buf.WriteString("/")
	buf.WriteString(meta.GetName())
	return buf.String()
}

func formatter(summarycache *summarycache.SummaryCache) types.Formatter {
	return func(request *types.APIRequest, resource *types.RawResource) {
		if resource.Schema == nil {
			return
		}

		gvr := attributes.GVR(resource.Schema)
		if gvr.Version == "" {
			return
		}

		meta, err := meta.Accessor(resource.APIObject.Object)
		if err != nil {
			return
		}
		selfLink := selfLink(gvr, meta)

		u := request.URLBuilder.RelativeToRoot(selfLink)
		resource.Links["view"] = u

		if _, ok := resource.Links["update"]; !ok && slice.ContainsString(resource.Schema.CollectionMethods, "PUT") {
			resource.Links["update"] = u
		}

		if _, ok := resource.Links["update"]; !ok && slice.ContainsString(resource.Schema.ResourceMethods, "blocked-PUT") {
			resource.Links["update"] = "blocked"
		}

		if _, ok := resource.Links["remove"]; !ok && slice.ContainsString(resource.Schema.ResourceMethods, "blocked-DELETE") {
			resource.Links["remove"] = "blocked"
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

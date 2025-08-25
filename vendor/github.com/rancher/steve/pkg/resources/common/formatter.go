package common

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/resources/virtual/common"
	"github.com/sirupsen/logrus"

	"github.com/rancher/steve/pkg/schema"
	metricsStore "github.com/rancher/steve/pkg/stores/metrics"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/steve/pkg/summarycache"
	"github.com/rancher/wrangler/v3/pkg/data"
	corecontrollers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/summary"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	schema2 "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"
)

type TemplateOptions struct {
	InSQLMode bool
}

func DefaultTemplate(clientGetter proxy.ClientGetter,
	summaryCache *summarycache.SummaryCache,
	asl accesscontrol.AccessSetLookup,
	namespaceCache corecontrollers.NamespaceCache,
	options TemplateOptions) schema.Template {
	return schema.Template{
		Store:     metricsStore.NewMetricsStore(proxy.NewProxyStore(clientGetter, summaryCache, asl, namespaceCache)),
		Formatter: formatter(summaryCache, asl, options),
	}
}

// DefaultTemplateForStore provides a default schema template which uses a provided, pre-initialized store. Primarily used when creating a Template that uses a Lasso SQL store internally.
func DefaultTemplateForStore(store types.Store,
	summaryCache *summarycache.SummaryCache,
	asl accesscontrol.AccessSetLookup,
	options TemplateOptions) schema.Template {
	return schema.Template{
		Store:     store,
		Formatter: formatter(summaryCache, asl, options),
	}
}

func selfLink(gvr schema2.GroupVersionResource, meta metav1.Object) (prefix string) {
	return buildBasePath(gvr, meta.GetNamespace(), meta.GetName())
}

func buildBasePath(gvr schema2.GroupVersionResource, namespace string, includeName string) string {
	buf := &strings.Builder{}

	if gvr.Group == "management.cattle.io" && gvr.Version == "v3" {
		buf.WriteString("/v1/")
		buf.WriteString(gvr.Group)
		buf.WriteString(".")
		buf.WriteString(gvr.Resource)
		if namespace != "" {
			buf.WriteString("/")
			buf.WriteString(namespace)
		}
	} else {
		if gvr.Group == "" {
			buf.WriteString("/api/v1/")
		} else {
			buf.WriteString("/apis/")
			buf.WriteString(gvr.Group)
			buf.WriteString("/")
			buf.WriteString(gvr.Version)
			buf.WriteString("/")
		}
		if namespace != "" {
			buf.WriteString("namespaces/")
			buf.WriteString(namespace)
			buf.WriteString("/")
		}
		buf.WriteString(gvr.Resource)
	}

	if includeName != "" {
		buf.WriteString("/")
		buf.WriteString(includeName)
	}

	return buf.String()
}

func formatter(summarycache common.SummaryCache, asl accesscontrol.AccessSetLookup, options TemplateOptions) types.Formatter {
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
		userInfo, ok := request.GetUserInfo()
		if !ok {
			return
		}
		accessSet := accesscontrol.AccessSetFromAPIRequest(request)
		if accessSet == nil {
			accessSet = asl.AccessFor(userInfo)
			if accessSet == nil {
				return
			}
		}
		hasUpdate := accessSet.Grants("update", gvr.GroupResource(), resource.APIObject.Namespace(), resource.APIObject.Name())
		hasDelete := accessSet.Grants("delete", gvr.GroupResource(), resource.APIObject.Namespace(), resource.APIObject.Name())
		hasPatch := accessSet.Grants("patch", gvr.GroupResource(), resource.APIObject.Namespace(), resource.APIObject.Name())

		selfLink := selfLink(gvr, meta)

		u := request.URLBuilder.RelativeToRoot(selfLink)
		resource.Links["view"] = u

		if hasUpdate {
			if attributes.DisallowMethods(resource.Schema)[http.MethodPut] {
				resource.Links["update"] = "blocked"
			}
		} else {
			delete(resource.Links, "update")
		}
		if hasDelete {
			if attributes.DisallowMethods(resource.Schema)[http.MethodDelete] {
				resource.Links["remove"] = "blocked"
			}
		} else {
			delete(resource.Links, "remove")
		}
		if hasPatch {
			if attributes.DisallowMethods(resource.Schema)[http.MethodPatch] {
				resource.Links["patch"] = "blocked"
			}
		} else {
			delete(resource.Links, "patch")
		}

		gvk := attributes.GVK(resource.Schema)
		if unstr, ok := resource.APIObject.Object.(*unstructured.Unstructured); ok {
			// with the sql cache, these were already added by the indexer. However, the sql cache
			// is only used for lists, so we need to re-add here for get/watch
			s, rel := summarycache.SummaryAndRelationship(unstr)
			data.PutValue(unstr.Object, map[string]interface{}{
				"name":          s.State,
				"error":         s.Error,
				"transitioning": s.Transitioning,
				"message":       strings.Join(s.Message, ":"),
			}, "metadata", "state")
			data.PutValue(unstr.Object, rel, "metadata", "relationships")

			summary.NormalizeConditions(unstr)

			includeFields(request, unstr)
			excludeFields(request, unstr)
			excludeValues(request, unstr)

			if options.InSQLMode {
				convertMetadataTimestampFields(request, gvk, unstr)
			}
		}

		if permsQuery := request.Query.Get("checkPermissions"); permsQuery != "" {
			ns := getNamespaceFromResource(resource.APIObject)
			permissions := map[string]map[string]string{}

			for _, res := range strings.Split(permsQuery, ",") {
				s := request.Schemas.LookupSchema(res)
				if s == nil {
					continue
				}
				gvr := attributes.GVR(s)
				gr := schema2.GroupResource{Group: gvr.Group, Resource: gvr.Resource}
				perms := map[string]string{}

				for _, verb := range []string{"create", "update", "delete", "list", "get", "watch", "patch"} {
					if accessSet.Grants(verb, gr, ns, "") {
						url := request.URLBuilder.RelativeToRoot(buildBasePath(gvr, ns, ""))
						perms[verb] = url
					}
				}
				if len(perms) > 0 {
					permissions[res] = perms
				}
			}

			if unstr, ok := resource.APIObject.Object.(*unstructured.Unstructured); ok {
				data.PutValue(unstr.Object, permissions, "resourcePermissions")
			}
		}
	}
}

func includeFields(request *types.APIRequest, unstr *unstructured.Unstructured) {
	if fields, ok := request.Query["include"]; ok {
		newObj := map[string]interface{}{}
		for _, f := range fields {
			fieldParts := strings.Split(f, ".")
			if val, ok := data.GetValue(unstr.Object, fieldParts...); ok {
				data.PutValue(newObj, val, fieldParts...)
			}
		}
		unstr.Object = newObj
	}
}

func excludeFields(request *types.APIRequest, unstr *unstructured.Unstructured) {
	if fields, ok := request.Query["exclude"]; ok {
		for _, f := range fields {
			fieldParts := strings.Split(f, ".")
			data.RemoveValue(unstr.Object, fieldParts...)
		}
	}
}

// convertMetadataTimestampFields updates metadata timestamp fields to ensure they remain fresh and human-readable when sent back
// to the client. Internally, fields are stored as Unix timestamps; on each request, we calculate the elapsed time since
// those timestamps by subtracting them from time.Now(), then format the resulting duration into a human-friendly string.
// This prevents cached durations (e.g. “2d” - 2 days) from becoming stale over time.
func convertMetadataTimestampFields(request *types.APIRequest, gvk schema2.GroupVersionKind, unstr *unstructured.Unstructured) {
	if request.Schema != nil {
		cols := GetColumnDefinitions(request.Schema)
		for _, col := range cols {
			gvkDateFields, gvkFound := DateFieldsByGVKBuiltins[gvk]
			if col.Type == "date" || (gvkFound && slices.Contains(gvkDateFields, col.Name)) {
				index := GetIndexValueFromString(col.Field)
				if index == -1 {
					logrus.Errorf("field index not found at column.Field struct variable: %s", col.Field)
					return
				}

				curValue, got, err := unstructured.NestedSlice(unstr.Object, "metadata", "fields")
				if err != nil {
					logrus.Warnf("failed to get metadata.fields slice from unstr.Object: %s", err.Error())
					return
				}

				if !got {
					logrus.Warnf("couldn't find metadata.fields at unstr.Object")
					return
				}

				timeValue, ok := curValue[index].(string)
				if !ok {
					logrus.Warnf("time field isn't a string")
					return
				}

				millis, err := strconv.ParseInt(timeValue, 10, 64)
				if err != nil {
					logrus.Warnf("convert timestamp value: %s failed with error: %s", timeValue, err.Error())
					return
				}

				timestamp := time.Unix(0, millis*int64(time.Millisecond))
				dur := time.Since(timestamp)

				humanDuration := duration.HumanDuration(dur)
				if humanDuration == "<invalid>" {
					logrus.Warnf("couldn't convert value %d into human duration for column %s", int64(dur), col.Name)
					return
				}

				curValue[index] = humanDuration
				if err := unstructured.SetNestedSlice(unstr.Object, curValue, "metadata", "fields"); err != nil {
					logrus.Errorf("failed to set value back to metadata.fields slice: %s", err.Error())
					return
				}
			}
		}
	}
}

func excludeValues(request *types.APIRequest, unstr *unstructured.Unstructured) {
	if values, ok := request.Query["excludeValues"]; ok {
		for _, f := range values {
			fieldParts := strings.Split(f, ".")
			fieldValues := data.GetValueN(unstr.Object, fieldParts...)
			if obj, ok := fieldValues.(map[string]interface{}); ok {
				for k := range obj {
					data.PutValue(unstr.Object, "", append(fieldParts, k)...)
				}
			}
		}
	}
}

func getNamespaceFromResource(obj types.APIObject) string {
	unstr, ok := obj.Object.(*unstructured.Unstructured)
	if !ok {
		return ""
	}

	// If we have a backingNamespace, use that
	if statusRaw, ok := unstr.Object["status"]; ok {
		if statusMap, ok := statusRaw.(map[string]interface{}); ok {
			if backingNamespace, ok := statusMap["backingNamespace"].(string); ok && backingNamespace != "" {
				return backingNamespace
			}
		}
	}

	// Otherwise, if the id has a slash, we will interpret that.
	// This is used to determine a project's namespace when there is no backingNamespace present.
	// For cases where there is no slash, we use the object's ID, which is the same as the namespace.
	return strings.Replace(obj.ID, "/", "-", 1)
}

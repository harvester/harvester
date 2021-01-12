package counts

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/clustercache"
	"github.com/rancher/wrangler/pkg/summary"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	schema2 "k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	ignore = map[string]bool{
		"count":   true,
		"schema":  true,
		"apiRoot": true,
	}
)

func Register(schemas *types.APISchemas, ccache clustercache.ClusterCache) {
	schemas.MustImportAndCustomize(Count{}, func(schema *types.APISchema) {
		schema.CollectionMethods = []string{http.MethodGet}
		schema.ResourceMethods = []string{http.MethodGet}
		schema.Attributes["access"] = accesscontrol.AccessListByVerb{
			"watch": accesscontrol.AccessList{
				{
					Namespace:    "*",
					ResourceName: "*",
				},
			},
		}
		schema.Store = &Store{
			ccache: ccache,
		}
	})
}

type Count struct {
	ID     string               `json:"id,omitempty"`
	Counts map[string]ItemCount `json:"counts"`
}

type Summary struct {
	Count         int            `json:"count,omitempty"`
	States        map[string]int `json:"states,omitempty"`
	Error         int            `json:"errors,omitempty"`
	Transitioning int            `json:"transitioning,omitempty"`
}

func (s *Summary) DeepCopy() *Summary {
	r := *s
	if r.States != nil {
		r.States = map[string]int{}
		for k := range s.States {
			r.States[k] = s.States[k]
		}
	}
	return &r
}

type ItemCount struct {
	Summary    Summary            `json:"summary,omitempty"`
	Namespaces map[string]Summary `json:"namespaces,omitempty"`
	Revision   int                `json:"-"`
}

func (i *ItemCount) DeepCopy() *ItemCount {
	r := *i
	r.Summary = *r.Summary.DeepCopy()
	if r.Namespaces != nil {
		r.Namespaces = map[string]Summary{}
		for k, v := range i.Namespaces {
			r.Namespaces[k] = *v.DeepCopy()
		}
	}
	return &r
}

type Store struct {
	empty.Store
	ccache clustercache.ClusterCache
}

func toAPIObject(c Count) types.APIObject {
	return types.APIObject{
		Type:   "count",
		ID:     c.ID,
		Object: c,
	}
}

func (s *Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	c := s.getCount(apiOp)
	return toAPIObject(c), nil
}

func (s *Store) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	c := s.getCount(apiOp)
	return types.APIObjectList{
		Objects: []types.APIObject{
			toAPIObject(c),
		},
	}, nil
}

func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan types.APIEvent, error) {
	var (
		result      = make(chan types.APIEvent, 100)
		counts      map[string]ItemCount
		gvkToSchema = map[schema2.GroupVersionKind]*types.APISchema{}
		countLock   sync.Mutex
	)

	go func() {
		<-apiOp.Context().Done()
		countLock.Lock()
		close(result)
		result = nil
		countLock.Unlock()
	}()

	counts = s.getCount(apiOp).Counts
	for id := range counts {
		schema := apiOp.Schemas.LookupSchema(id)
		if schema == nil {
			continue
		}

		gvkToSchema[attributes.GVK(schema)] = schema
	}

	onChange := func(add bool, gvk schema2.GroupVersionKind, _ string, obj, oldObj runtime.Object) error {
		countLock.Lock()
		defer countLock.Unlock()

		if result == nil {
			return nil
		}

		schema := gvkToSchema[gvk]
		if schema == nil {
			return nil
		}

		_, namespace, revision, summary, ok := getInfo(obj)
		if !ok {
			return nil
		}

		itemCount := counts[schema.ID]
		if revision <= itemCount.Revision {
			return nil
		}

		if oldObj != nil {
			if _, _, _, oldSummary, ok := getInfo(oldObj); ok {
				if oldSummary.Transitioning == summary.Transitioning &&
					oldSummary.Error == summary.Error &&
					simpleState(oldSummary) == simpleState(summary) {
					return nil
				}
				itemCount = removeCounts(itemCount, namespace, oldSummary)
				itemCount = addCounts(itemCount, namespace, summary)
			} else {
				return nil
			}
		} else if add {
			itemCount = addCounts(itemCount, namespace, summary)
		} else {
			itemCount = removeCounts(itemCount, namespace, summary)
		}

		counts[schema.ID] = itemCount
		countsCopy := map[string]ItemCount{}
		for k, v := range counts {
			countsCopy[k] = *v.DeepCopy()
		}

		result <- types.APIEvent{
			Name:         "resource.change",
			ResourceType: "counts",
			Object: toAPIObject(Count{
				ID:     "count",
				Counts: countsCopy,
			}),
		}

		return nil
	}

	s.ccache.OnAdd(apiOp.Context(), func(gvk schema2.GroupVersionKind, key string, obj runtime.Object) error {
		return onChange(true, gvk, key, obj, nil)
	})
	s.ccache.OnChange(apiOp.Context(), func(gvk schema2.GroupVersionKind, key string, obj, oldObj runtime.Object) error {
		return onChange(true, gvk, key, obj, oldObj)
	})
	s.ccache.OnRemove(apiOp.Context(), func(gvk schema2.GroupVersionKind, key string, obj runtime.Object) error {
		return onChange(false, gvk, key, obj, nil)
	})

	return buffer(result), nil
}

func (s *Store) schemasToWatch(apiOp *types.APIRequest) (result []*types.APISchema) {
	for _, schema := range apiOp.Schemas.Schemas {
		if ignore[schema.ID] {
			continue
		}

		if schema.Store == nil {
			continue
		}

		if apiOp.AccessControl.CanList(apiOp, schema) != nil {
			continue
		}

		if apiOp.AccessControl.CanWatch(apiOp, schema) != nil {
			continue
		}

		result = append(result, schema)
	}

	return
}

func getInfo(obj interface{}) (name string, namespace string, revision int, summaryResult summary.Summary, ok bool) {
	r, ok := obj.(runtime.Object)
	if !ok {
		return "", "", 0, summaryResult, false
	}

	meta, err := meta.Accessor(r)
	if err != nil {
		return "", "", 0, summaryResult, false
	}

	revision, err = strconv.Atoi(meta.GetResourceVersion())
	if err != nil {
		return "", "", 0, summaryResult, false
	}

	summaryResult = summary.Summarize(r)
	return meta.GetName(), meta.GetNamespace(), revision, summaryResult, true
}

func removeCounts(itemCount ItemCount, ns string, summary summary.Summary) ItemCount {
	itemCount.Summary = removeSummary(itemCount.Summary, summary)
	if ns != "" {
		itemCount.Namespaces[ns] = removeSummary(itemCount.Namespaces[ns], summary)
	}
	return itemCount
}

func addCounts(itemCount ItemCount, ns string, summary summary.Summary) ItemCount {
	itemCount.Summary = addSummary(itemCount.Summary, summary)
	if ns != "" {
		itemCount.Namespaces[ns] = addSummary(itemCount.Namespaces[ns], summary)
	}
	return itemCount
}

func removeSummary(counts Summary, summary summary.Summary) Summary {
	counts.Count--
	if summary.Transitioning {
		counts.Transitioning--
	}
	if summary.Error {
		counts.Error--
	}
	if simpleState(summary) != "" {
		if counts.States == nil {
			counts.States = map[string]int{}
		}
		counts.States[simpleState(summary)] -= 1
	}
	return counts
}

func addSummary(counts Summary, summary summary.Summary) Summary {
	counts.Count++
	if summary.Transitioning {
		counts.Transitioning++
	}
	if summary.Error {
		counts.Error++
	}
	if simpleState(summary) != "" {
		if counts.States == nil {
			counts.States = map[string]int{}
		}
		counts.States[simpleState(summary)] += 1
	}
	return counts
}

func simpleState(summary summary.Summary) string {
	if summary.Error {
		return "error"
	} else if summary.Transitioning {
		return "in-progress"
	}
	return ""
}

func (s *Store) getCount(apiOp *types.APIRequest) Count {
	counts := map[string]ItemCount{}

	for _, schema := range s.schemasToWatch(apiOp) {
		gvk := attributes.GVK(schema)
		access, _ := attributes.Access(schema).(accesscontrol.AccessListByVerb)

		rev := 0
		itemCount := ItemCount{
			Namespaces: map[string]Summary{},
		}

		all := access.Grants("list", "*", "*")

		for _, obj := range s.ccache.List(gvk) {
			name, ns, revision, summary, ok := getInfo(obj)
			if !ok {
				continue
			}

			if !all && !access.Grants("list", ns, name) && !access.Grants("get", ns, name) {
				continue
			}

			if revision > rev {
				rev = revision
			}

			itemCount = addCounts(itemCount, ns, summary)
		}

		itemCount.Revision = rev
		counts[schema.ID] = itemCount
	}

	return Count{
		ID:     "count",
		Counts: counts,
	}
}

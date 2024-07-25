// Package partition implements a store with parallel partitioning of data
// so that segmented data can be concurrently collected and returned as a single data set.
package partition

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/stores/partition/listprocessor"
	corecontrollers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
)

const (
	// Number of list request entries to save before cache replacement.
	// Not related to the total size in memory of the cache, as any item could take any amount of memory.
	cacheSizeEnv     = "CATTLE_REQUEST_CACHE_SIZE_INT"
	defaultCacheSize = 1000
	// Set to "false" to enable list request caching.
	cacheDisableEnv = "CATTLE_REQUEST_CACHE_DISABLED"
)

// Partitioner is an interface for interacting with partitions.
type Partitioner interface {
	Lookup(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) (Partition, error)
	All(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) ([]Partition, error)
	Store(apiOp *types.APIRequest, partition Partition) (UnstructuredStore, error)
}

// Store implements types.Store for partitions.
type Store struct {
	Partitioner    Partitioner
	listCache      *cache.LRUExpireCache
	asl            accesscontrol.AccessSetLookup
	namespaceCache corecontrollers.NamespaceCache
}

// NewStore creates a types.Store implementation with a partitioner and an LRU expiring cache for list responses.
func NewStore(partitioner Partitioner, asl accesscontrol.AccessSetLookup, namespaceCache corecontrollers.NamespaceCache) *Store {
	cacheSize := defaultCacheSize
	if v := os.Getenv(cacheSizeEnv); v != "" {
		sizeInt, err := strconv.Atoi(v)
		if err == nil {
			cacheSize = sizeInt
		}
	}
	s := &Store{
		Partitioner:    partitioner,
		asl:            asl,
		namespaceCache: namespaceCache,
	}
	if v := os.Getenv(cacheDisableEnv); v == "false" {
		s.listCache = cache.NewLRUExpireCache(cacheSize)
	}
	return s
}

type cacheKey struct {
	chunkSize    int
	resume       string
	filters      string
	sort         string
	pageSize     int
	accessID     string
	resourcePath string
	revision     string
}

// UnstructuredStore is like types.Store but deals in k8s unstructured objects instead of apiserver types.
type UnstructuredStore interface {
	ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error)
	List(apiOp *types.APIRequest, schema *types.APISchema) (*unstructured.UnstructuredList, []types.Warning, error)
	Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (*unstructured.Unstructured, []types.Warning, error)
	Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (*unstructured.Unstructured, []types.Warning, error)
	Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error)
	Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan watch.Event, error)
}

func (s *Store) getStore(apiOp *types.APIRequest, schema *types.APISchema, verb, id string) (UnstructuredStore, error) {
	p, err := s.Partitioner.Lookup(apiOp, schema, verb, id)
	if err != nil {
		return nil, err
	}

	return s.Partitioner.Store(apiOp, p)
}

// Delete deletes an object from a store.
func (s *Store) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	target, err := s.getStore(apiOp, schema, "delete", id)
	if err != nil {
		return types.APIObject{}, err
	}

	obj, warnings, err := target.Delete(apiOp, schema, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return ToAPI(schema, obj, warnings), nil
}

// ByID looks up a single object by its ID.
func (s *Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	target, err := s.getStore(apiOp, schema, "get", id)
	if err != nil {
		return types.APIObject{}, err
	}

	obj, warnings, err := target.ByID(apiOp, schema, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return ToAPI(schema, obj, warnings), nil
}

func (s *Store) listPartition(ctx context.Context, apiOp *types.APIRequest, schema *types.APISchema, partition Partition,
	cont string, revision string, limit int) (*unstructured.UnstructuredList, []types.Warning, error) {
	store, err := s.Partitioner.Store(apiOp, partition)
	if err != nil {
		return nil, nil, err
	}

	req := apiOp.Clone()
	req.Request = req.Request.Clone(ctx)

	values := req.Request.URL.Query()
	values.Set("continue", cont)
	if revision != "" && cont == "" {
		values.Set("resourceVersion", revision)
		values.Set("resourceVersionMatch", "Exact") // supported since k8s 1.19
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	} else {
		values.Del("limit")
	}
	req.Request.URL.RawQuery = values.Encode()

	return store.List(req, schema)
}

// List returns a list of objects across all applicable partitions.
// If pagination parameters are used, it returns a segment of the list.
func (s *Store) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	var (
		result types.APIObjectList
	)

	partitions, err := s.Partitioner.All(apiOp, schema, "list", "")
	if err != nil {
		return result, err
	}

	lister := ParallelPartitionLister{
		Lister: func(ctx context.Context, partition Partition, cont string, revision string, limit int) (*unstructured.UnstructuredList, []types.Warning, error) {
			return s.listPartition(ctx, apiOp, schema, partition, cont, revision, limit)
		},
		Concurrency: 3,
		Partitions:  partitions,
	}

	opts := listprocessor.ParseQuery(apiOp)

	var list []unstructured.Unstructured
	var key cacheKey
	if s.listCache != nil {
		key, err = s.getCacheKey(apiOp, opts)
		if err != nil {
			return result, err
		}

		if key.revision != "" {
			cachedList, ok := s.listCache.Get(key)
			if ok {
				logrus.Tracef("found cached list for query %s?%s", apiOp.Request.URL.Path, apiOp.Request.URL.RawQuery)
				list = cachedList.(*unstructured.UnstructuredList).Items
				result.Continue = cachedList.(*unstructured.UnstructuredList).GetContinue()
				result.Revision = key.revision
			}
		}
	}
	if list == nil { // did not look in cache or was not found in cache
		stream, err := lister.List(apiOp.Context(), opts.ChunkSize, opts.Resume, opts.Revision)
		if err != nil {
			return result, err
		}
		list = listprocessor.FilterList(stream, opts.Filters)
		// Check for any errors returned during the parallel listing requests.
		// We don't want to cache the list or bother with further processing if the list is empty or corrupt.
		// FilterList guarantees that the stream has been consumed and the error is populated if there is any.
		if lister.Err() != nil {
			return result, lister.Err()
		}
		list = listprocessor.SortList(list, opts.Sort)
		result.Revision = lister.Revision()
		listToCache := &unstructured.UnstructuredList{
			Items: list,
		}
		list = listprocessor.FilterByProjectsAndNamespaces(list, opts.ProjectsOrNamespaces, s.namespaceCache)
		c := lister.Continue()
		if c != "" {
			listToCache.SetContinue(c)
		}
		if s.listCache != nil {
			key.revision = result.Revision
			s.listCache.Add(key, listToCache, 30*time.Minute)
		}
		result.Continue = lister.Continue()
	}
	result.Count = len(list)
	list, pages := listprocessor.PaginateList(list, opts.Pagination)

	for _, item := range list {
		item := item.DeepCopy()
		result.Objects = append(result.Objects, ToAPI(schema, item, nil))
	}

	result.Pages = pages
	return result, lister.Err()
}

// getCacheKey returns a hashable struct identifying a unique user and request.
func (s *Store) getCacheKey(apiOp *types.APIRequest, opts *listprocessor.ListOptions) (cacheKey, error) {
	user, ok := request.UserFrom(apiOp.Request.Context())
	if !ok {
		return cacheKey{}, fmt.Errorf("could not find user in request")
	}
	filters := ""
	for _, f := range opts.Filters {
		filters = filters + f.String()
	}
	return cacheKey{
		chunkSize:    opts.ChunkSize,
		resume:       opts.Resume,
		filters:      filters,
		sort:         opts.Sort.String(),
		pageSize:     opts.Pagination.PageSize(),
		accessID:     s.asl.AccessFor(user).ID,
		resourcePath: apiOp.Request.URL.Path,
		revision:     opts.Revision,
	}, nil
}

// Create creates a single object in the store.
func (s *Store) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	target, err := s.getStore(apiOp, schema, "create", "")
	if err != nil {
		return types.APIObject{}, err
	}

	obj, warnings, err := target.Create(apiOp, schema, data)
	if err != nil {
		return types.APIObject{}, err
	}
	return ToAPI(schema, obj, warnings), nil
}

// Update updates a single object in the store.
func (s *Store) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	target, err := s.getStore(apiOp, schema, "update", id)
	if err != nil {
		return types.APIObject{}, err
	}

	obj, warnings, err := target.Update(apiOp, schema, data, id)
	if err != nil {
		return types.APIObject{}, err
	}
	return ToAPI(schema, obj, warnings), nil
}

// Watch returns a channel of events for a list or resource.
func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan types.APIEvent, error) {
	partitions, err := s.Partitioner.All(apiOp, schema, "watch", wr.ID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(apiOp.Context())
	apiOp = apiOp.Clone().WithContext(ctx)

	eg := errgroup.Group{}
	response := make(chan types.APIEvent)

	for _, partition := range partitions {
		store, err := s.Partitioner.Store(apiOp, partition)
		if err != nil {
			cancel()
			return nil, err
		}

		eg.Go(func() error {
			defer cancel()
			c, err := store.Watch(apiOp, schema, wr)
			if err != nil {
				return err
			}
			for i := range c {
				response <- ToAPIEvent(apiOp, schema, i)
			}
			return nil
		})
	}

	go func() {
		defer close(response)
		<-ctx.Done()
		eg.Wait()
		cancel()
	}()

	return response, nil
}

func ToAPI(schema *types.APISchema, obj runtime.Object, warnings []types.Warning) types.APIObject {
	if obj == nil || reflect.ValueOf(obj).IsNil() {
		return types.APIObject{}
	}

	if unstr, ok := obj.(*unstructured.Unstructured); ok {
		obj = moveToUnderscore(unstr)
	}

	apiObject := types.APIObject{
		Type:   schema.ID,
		Object: obj,
	}

	m, err := meta.Accessor(obj)
	if err != nil {
		return apiObject
	}

	id := m.GetName()
	ns := m.GetNamespace()
	if ns != "" {
		id = fmt.Sprintf("%s/%s", ns, id)
	}

	apiObject.ID = id
	apiObject.Warnings = warnings
	return apiObject
}

func moveToUnderscore(obj *unstructured.Unstructured) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}

	for k := range types.ReservedFields {
		v, ok := obj.Object[k]
		if ok {
			delete(obj.Object, k)
			obj.Object["_"+k] = v
		}
	}

	return obj
}

func ToAPIEvent(apiOp *types.APIRequest, schema *types.APISchema, event watch.Event) types.APIEvent {
	name := types.ChangeAPIEvent
	switch event.Type {
	case watch.Deleted:
		name = types.RemoveAPIEvent
	case watch.Added:
		name = types.CreateAPIEvent
	case watch.Error:
		name = "resource.error"
	}

	apiEvent := types.APIEvent{
		Name: name,
	}

	if event.Type == watch.Error {
		status, _ := event.Object.(*metav1.Status)
		apiEvent.Error = fmt.Errorf(status.Message)
		return apiEvent
	}

	apiEvent.Object = ToAPI(schema, event.Object, nil)

	m, err := meta.Accessor(event.Object)
	if err != nil {
		return apiEvent
	}

	apiEvent.Revision = m.GetResourceVersion()
	return apiEvent
}

package generic

import (
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

// CacheInterface is an interface for Object retrieval from memory.
type CacheInterface[T runtime.Object] interface {
	// Get returns the resources with the specified name in the given namespace from the cache.
	Get(namespace, name string) (T, error)

	// List will attempt to find resources in the given namespace from the Cache.
	List(namespace string, selector labels.Selector) ([]T, error)

	// AddIndexer adds  a new Indexer to the cache with the provided name.
	// If you call this after you already have data in the store, the results are undefined.
	AddIndexer(indexName string, indexer Indexer[T])

	// GetByIndex returns the stored objects whose set of indexed values
	// for the named index includes the given indexed value.
	GetByIndex(indexName, key string) ([]T, error)
}

// NonNamespacedCacheInterface is an interface for non namespaced Object retrieval from memory.
type NonNamespacedCacheInterface[T runtime.Object] interface {
	// Get returns the resources with the specified name from the cache.
	Get(name string) (T, error)

	// List will attempt to find resources from the Cache.
	List(selector labels.Selector) ([]T, error)

	// AddIndexer adds  a new Indexer to the cache with the provided name.
	// If you call this after you already have data in the store, the results are undefined.
	AddIndexer(indexName string, indexer Indexer[T])

	// GetByIndex returns the stored objects whose set of indexed values
	// for the named index includes the given indexed value.
	GetByIndex(indexName, key string) ([]T, error)
}

// Cache is a object cache stored in memory for objects of type T.
type Cache[T runtime.Object] struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

// NonNamespacedCache is a Cache for objects of type T that are not namespaced.
type NonNamespacedCache[T runtime.Object] struct {
	CacheInterface[T]
}

// Get returns the resources with the specified name in the given namespace from the cache.
func (c *Cache[T]) Get(namespace, name string) (T, error) {
	var nilObj T
	key := name
	if namespace != metav1.NamespaceAll {
		key = namespace + "/" + key
	}
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		return nilObj, err
	}
	if !exists {
		return nilObj, errors.NewNotFound(c.resource, name)
	}
	return obj.(T), nil
}

// List will attempt to find resources in the given namespace from the Cache.
func (c *Cache[T]) List(namespace string, selector labels.Selector) (ret []T, err error) {
	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(T))
	})

	return ret, err
}

// AddIndexer adds  a new Indexer to the cache with the provided name.
// If you call this after you already have data in the store, the results are undefined.
func (c *Cache[T]) AddIndexer(indexName string, indexer Indexer[T]) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(T))
		},
	}))
}

// GetByIndex returns the stored objects whose set of indexed values
// for the named index includes the given indexed value.
func (c *Cache[T]) GetByIndex(indexName, key string) (result []T, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]T, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(T))
	}
	return result, nil
}

// Get calls Cache.Get(...) with an empty namespace parameter.
func (c *NonNamespacedCache[T]) Get(name string) (T, error) {
	return c.CacheInterface.Get(metav1.NamespaceAll, name)
}

// Get calls Cache.List(...) with an empty namespace parameter.
func (c *NonNamespacedCache[T]) List(selector labels.Selector) (ret []T, err error) {
	return c.CacheInterface.List(metav1.NamespaceAll, selector)
}

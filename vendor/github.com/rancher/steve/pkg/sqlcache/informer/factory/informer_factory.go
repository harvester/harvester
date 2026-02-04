/*
Package factory provides a cache factory for the sql-based cache.
*/
package factory

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rancher/lasso/pkg/log"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/encryption"
	"github.com/rancher/steve/pkg/sqlcache/informer"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

// EncryptAllEnvVar is set to "true" if users want all types' data blobs to be encrypted in SQLite
// otherwise only variables in defaultEncryptedResourceTypes will have their blobs encrypted
const EncryptAllEnvVar = "CATTLE_ENCRYPT_CACHE_ALL"

// CacheFactory builds Informer instances and keeps a cache of instances it created
type CacheFactory struct {
	wg         wait.Group
	dbClient   db.Client
	stopCh     chan struct{}
	mutex      sync.RWMutex
	encryptAll bool

	defaultMaximumEventsCount int
	perGVKMaximumEventsCount  map[schema.GroupVersionKind]int

	newInformer newInformer

	informers      map[schema.GroupVersionKind]*guardedInformer
	informersMutex sync.Mutex
}

type guardedInformer struct {
	informer *informer.Informer
	mutex    *sync.Mutex
}

type newInformer func(ctx context.Context, client dynamic.ResourceInterface, fields [][]string, transform cache.TransformFunc, gvk schema.GroupVersionKind, db db.Client, shouldEncrypt bool, namespace bool, watchable bool, maxEventsCount int) (*informer.Informer, error)

type Cache struct {
	informer.ByOptionsLister
}

var defaultEncryptedResourceTypes = map[schema.GroupVersionKind]struct{}{
	{
		Version: "v1",
		Kind:    "Secret",
	}: {},
	{
		Group:   "management.cattle.io",
		Version: "v3",
		Kind:    "Token",
	}: {},
}

type CacheFactoryOptions struct {
	// DefaultMaximumEventsCount is the maximum number of events to keep in
	// the events table by default.
	//
	// Use PerGVKMaximumEventsCount if you want to set a different value for
	// a specific GVK.
	//
	// A value of 0 means no limits.
	DefaultMaximumEventsCount int
	// PerGVKMaximumEventsCount is the maximum number of events to keep in
	// the events table for specific GVKs.
	//
	// A value of 0 means no limits.
	PerGVKMaximumEventsCount map[schema.GroupVersionKind]int
}

// NewCacheFactory returns an informer factory instance
// This is currently called from steve via initial calls to `s.cacheFactory.CacheFor(...)`
func NewCacheFactory(opts CacheFactoryOptions) (*CacheFactory, error) {
	m, err := encryption.NewManager()
	if err != nil {
		return nil, err
	}
	dbClient, _, err := db.NewClient(nil, m, m, false)
	if err != nil {
		return nil, err
	}
	return &CacheFactory{
		wg:         wait.Group{},
		stopCh:     make(chan struct{}),
		encryptAll: os.Getenv(EncryptAllEnvVar) == "true",
		dbClient:   dbClient,

		defaultMaximumEventsCount: opts.DefaultMaximumEventsCount,
		perGVKMaximumEventsCount:  opts.PerGVKMaximumEventsCount,

		newInformer: informer.NewInformer,
		informers:   map[schema.GroupVersionKind]*guardedInformer{},
	}, nil
}

// CacheFor returns an informer for given GVK, using sql store indexed with fields, using the specified client. For virtual fields, they must be added by the transform function
// and specified by fields to be used for later fields.
func (f *CacheFactory) CacheFor(ctx context.Context, fields [][]string, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool, watchable bool) (Cache, error) {
	// First of all block Reset() until we are done
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// Second, check if the informer and its accompanying informer-specific mutex exist already in the informers cache
	// If not, start by creating such informer-specific mutex. That is used later to ensure no two goroutines create
	// informers for the same GVK at the same type
	f.informersMutex.Lock()
	// Note: the informers cache is protected by informersMutex, which we don't want to hold for very long because
	// that blocks CacheFor for other GVKs, hence not deferring unlock here
	gi, ok := f.informers[gvk]
	if !ok {
		gi = &guardedInformer{
			informer: nil,
			mutex:    &sync.Mutex{},
		}
		f.informers[gvk] = gi
	}
	f.informersMutex.Unlock()

	// At this point an informer-specific mutex (gi.mutex) is guaranteed to exist. Lock it
	gi.mutex.Lock()
	defer gi.mutex.Unlock()

	// Then: if the informer really was not created yet (first time here or previous times have errored out)
	// actually create the informer
	if gi.informer == nil {
		start := time.Now()
		log.Debugf("CacheFor STARTS creating informer for %v", gvk)
		defer func() {
			log.Debugf("CacheFor IS DONE creating informer for %v (took %v)", gvk, time.Now().Sub(start))
		}()

		_, encryptResourceAlways := defaultEncryptedResourceTypes[gvk]
		shouldEncrypt := f.encryptAll || encryptResourceAlways
		maxEventsCount := f.getMaximumEventsCount(gvk)
		i, err := f.newInformer(ctx, client, fields, transform, gvk, f.dbClient, shouldEncrypt, namespaced, watchable, maxEventsCount)
		if err != nil {
			return Cache{}, err
		}

		err = i.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
			if !watchable && errors.IsMethodNotSupported(err) {
				// expected, continue without logging
				return
			}
			cache.DefaultWatchErrorHandler(r, err)
		})
		if err != nil {
			return Cache{}, err
		}

		f.wg.StartWithChannel(f.stopCh, i.Run)

		gi.informer = i
	}

	if !cache.WaitForCacheSync(f.stopCh, gi.informer.HasSynced) {
		return Cache{}, fmt.Errorf("failed to sync SQLite Informer cache for GVK %v", gvk)
	}

	// At this point the informer is ready, return it
	return Cache{ByOptionsLister: gi.informer}, nil
}

func (f *CacheFactory) getMaximumEventsCount(gvk schema.GroupVersionKind) int {
	if maxCount, ok := f.perGVKMaximumEventsCount[gvk]; ok {
		return maxCount
	}
	return f.defaultMaximumEventsCount
}

// Reset closes the stopCh which stops any running informers, assigns a new stopCh, resets the GVK-informer cache, and resets
// the database connection which wipes any current sqlite database at the default location.
func (f *CacheFactory) Reset() error {
	if f.dbClient == nil {
		// nothing to reset
		return nil
	}

	// first of all wait until all CacheFor() calls that create new informers are finished. Also block any new ones
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// now that we are alone, stop all informers created until this point
	close(f.stopCh)
	f.stopCh = make(chan struct{})
	f.wg.Wait()

	// and get rid of all references to those informers and their mutexes
	f.informersMutex.Lock()
	defer f.informersMutex.Unlock()
	f.informers = make(map[schema.GroupVersionKind]*guardedInformer)

	// finally, reset the DB connection
	_, err := f.dbClient.NewConnection(false)
	if err != nil {
		return err
	}

	return nil
}

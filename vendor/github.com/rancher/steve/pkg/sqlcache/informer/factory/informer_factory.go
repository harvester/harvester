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
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
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
	dbClient db.Client

	// ctx determines when informers need to stop
	ctx    context.Context
	cancel context.CancelFunc

	encryptAll bool

	gcInterval  time.Duration
	gcKeepCount int

	newInformer newInformer

	informers      map[schema.GroupVersionKind]*guardedInformer
	informersMutex sync.Mutex
}

type guardedInformer struct {
	informer *informer.Informer
	// informerMutex ensures informer is only set by one goroutine even if
	// multiple concurrent calls to CacheFor are made
	informerMutex *sync.Mutex

	// stopMutex ensures no CacheFor call can be made for a given GVK when
	// a Stop call is ongoing.
	//
	// CacheFactory.informersMutex is not enough because part of the code
	// might still have an old cache from a previous CacheFor call.
	stopMutex *sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     wait.Group
}

type newInformer func(ctx context.Context, client dynamic.ResourceInterface, fields [][]string, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, gvk schema.GroupVersionKind, db db.Client, shouldEncrypt bool, typeGuidance map[string]string, namespace bool, watchable bool, gcInterval time.Duration, gcKeepCount int) (*informer.Informer, error)

type Cache struct {
	informer.ByOptionsLister
	gvk schema.GroupVersionKind
	ctx context.Context
	gi  *guardedInformer
}

// Context gives the context of the factory that created this cache.
//
// The context is canceled when the cache is stopped (eg: when the CRD column definition changes)
func (c *Cache) Context() context.Context {
	return c.ctx
}

func (c *Cache) GVK() schema.GroupVersionKind {
	return c.gvk
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
	// GCInterval is how often to run the garbage collection
	GCInterval time.Duration
	// GCKeepCount is how many events to keep in _events table when gc runs
	GCKeepCount int
}

// NewCacheFactory returns an informer factory instance
// This is currently called from steve via initial calls to `s.cacheFactory.CacheFor(...)`
func NewCacheFactory(opts CacheFactoryOptions) (*CacheFactory, error) {
	return NewCacheFactoryWithContext(context.Background(), opts)
}

func NewCacheFactoryWithContext(ctx context.Context, opts CacheFactoryOptions) (*CacheFactory, error) {
	m, err := encryption.NewManager()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	dbClient, _, err := db.NewClient(ctx, nil, m, m, false)
	if err != nil {
		cancel()
		return nil, err
	}
	return &CacheFactory{
		ctx:    ctx,
		cancel: cancel,

		encryptAll: os.Getenv(EncryptAllEnvVar) == "true",
		dbClient:   dbClient,

		gcInterval:  opts.GCInterval,
		gcKeepCount: opts.GCKeepCount,

		newInformer: informer.NewInformer,
		informers:   map[schema.GroupVersionKind]*guardedInformer{},
	}, nil
}

// CacheFor returns an informer for given GVK, using sql store indexed with fields, using the specified client. For virtual fields, they must be added by the transform function
// and specified by fields to be used for later fields.
//
// There's a few context.Context involved. Here's the hierarchy:
//   - ctx is the context of a request (eg: [net/http.Request.Context]). It is canceled when the request finishes (eg: the client timed out or canceled the request)
//   - [CacheFactory.ctx] is the context for the cache factory. This is canceled when we no longer need the cache factory
//   - [guardedInformer.ctx] is the context for a single cache. Its parent is the [CacheFactory.ctx] so that all caches stops when the cache factory stop. We need
//     a context for a single cache to be able to stop that cache (eg: on schema refresh) without impacting the other caches.
//
// Don't forget to call DoneWithCache with the given informer once done with it.
func (f *CacheFactory) CacheFor(ctx context.Context, fields [][]string, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, typeGuidance map[string]string, namespaced bool, watchable bool) (*Cache, error) {
	// Second, check if the informer and its accompanying informer-specific mutex exist already in the informers cache
	// If not, start by creating such informer-specific mutex. That is used later to ensure no two goroutines create
	// informers for the same GVK at the same type
	f.informersMutex.Lock()
	// Note: the informers cache is protected by informersMutex, which we don't want to hold for very long because
	// that blocks CacheFor for other GVKs, hence not deferring unlock here
	gi, ok := f.informers[gvk]
	if !ok {
		giCtx, giCancel := context.WithCancel(f.ctx)
		gi = &guardedInformer{
			informer:      nil,
			informerMutex: &sync.Mutex{},
			stopMutex:     &sync.RWMutex{},
			ctx:           giCtx,
			cancel:        giCancel,
		}
		f.informers[gvk] = gi
	}
	f.informersMutex.Unlock()

	// Prevent Stop() to be called for that GVK
	gi.stopMutex.RLock()

	gvkCache, err := f.cacheForLocked(ctx, gi, fields, externalUpdateInfo, selfUpdateInfo, transform, client, gvk, typeGuidance, namespaced, watchable)
	if err != nil {
		gi.stopMutex.RUnlock()
		return nil, err
	}
	return gvkCache, nil
}

func (f *CacheFactory) cacheForLocked(ctx context.Context, gi *guardedInformer, fields [][]string, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, typeGuidance map[string]string, namespaced bool, watchable bool) (*Cache, error) {
	// At this point an informer-specific mutex (gi.mutex) is guaranteed to exist. Lock it
	gi.informerMutex.Lock()

	// Then: if the informer really was not created yet (first time here or previous times have errored out)
	// actually create the informer
	if gi.informer == nil {
		start := time.Now()
		log.Infof("CacheFor STARTS creating informer for %v", gvk)
		defer func() {
			log.Infof("CacheFor IS DONE creating informer for %v (took %v)", gvk, time.Since(start))
		}()

		_, encryptResourceAlways := defaultEncryptedResourceTypes[gvk]
		shouldEncrypt := f.encryptAll || encryptResourceAlways
		// In non-test code this invokes pkg/sqlcache/informer/informer.go: NewInformer()
		// search for "func NewInformer(ctx"
		i, err := f.newInformer(gi.ctx, client, fields, externalUpdateInfo, selfUpdateInfo, transform, gvk, f.dbClient, shouldEncrypt, typeGuidance, namespaced, watchable, f.gcInterval, f.gcKeepCount)
		if err != nil {
			gi.informerMutex.Unlock()
			return nil, err
		}

		err = i.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
			if !watchable && errors.IsMethodNotSupported(err) {
				// expected, continue without logging
				return
			}
			cache.DefaultWatchErrorHandler(gi.ctx, r, err)
		})
		if err != nil {
			gi.informerMutex.Unlock()
			return nil, err
		}

		gi.wg.StartWithChannel(gi.ctx.Done(), i.Run)

		gi.informer = i
	}
	gi.informerMutex.Unlock()

	// We don't want to get stuck in WaitForCachesSync if the request from
	// the client has been canceled.
	waitCh := make(chan struct{}, 1)
	go func() {
		select {
		case <-ctx.Done():
		case <-gi.ctx.Done():
		}
		close(waitCh)
	}()

	if !cache.WaitForCacheSync(waitCh, gi.informer.HasSynced) {
		if gi.ctx.Err() != nil {
			return nil, fmt.Errorf("cache context canceled while waiting for SQL cache sync for %v: %w", gvk, gi.ctx.Err())
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request context canceled while waiting for SQL cache sync for %v: %w", gvk, ctx.Err())
		}
		return nil, fmt.Errorf("failed to sync SQLite Informer cache for GVK %v", gvk)
	}

	// At this point the informer is ready, return it
	return &Cache{ByOptionsLister: gi.informer, gvk: gvk, ctx: gi.ctx, gi: gi}, nil
}

// DoneWithCache must be called for every successful CacheFor call. The Cache should
// no longer be used after DoneWithCache is called.
//
// This ensures that there aren't any inflight list requests while we are resetting the database.
func (f *CacheFactory) DoneWithCache(cache *Cache) {
	if cache == nil {
		return
	}

	cache.gi.stopMutex.RUnlock()
}

// Stop cancels ctx which stops any running informers, assigns a new ctx, resets the GVK-informer cache, and resets
// the database connection which wipes any current sqlite database at the default location.
func (f *CacheFactory) Stop(gvk schema.GroupVersionKind) error {
	if f.dbClient == nil {
		// nothing to reset
		return nil
	}

	f.informersMutex.Lock()
	defer f.informersMutex.Unlock()

	gi, ok := f.informers[gvk]
	if !ok {
		return nil
	}
	delete(f.informers, gvk)

	// We must stop informers here to unblock those stuck in WaitForCacheSync
	// which is blocking DoneWithCache call.
	gi.cancel()

	// Prevent other CacheFor calls for that GVK
	gi.stopMutex.Lock()
	defer gi.stopMutex.Unlock()

	// Wait for all informers to have exited
	gi.wg.Wait()

	// Since we hold the lock on gi.stopMutex, we do not need to also hold
	// onto gi.informersMutex
	if gi.informer != nil {
		// DropAll needs its own context because the context from the informer
		// is canceled
		err := gi.informer.DropAll(context.Background())
		if err != nil {
			return fmt.Errorf("dropall %q: %w", gvk, err)
		}
	}

	return nil
}

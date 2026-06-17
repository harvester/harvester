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
	"github.com/rancher/steve/pkg/metrics"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/encryption"
	"github.com/rancher/steve/pkg/sqlcache/informer"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	gcKeepCount int

	newInformer newInformer

	informers      map[schema.GroupVersionKind]*guardedInformer
	informersMutex sync.Mutex
}

type guardedInformer struct {
	// This mutex ensures the initialization of informer, as well as controlling a graceful shutdown
	mutex    sync.RWMutex
	informer *informer.Informer
	// informerDone is non-nil when the informer is started, and closed when it finishes
	informerDone chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	// wg represents all active cache users, and is decreased when DoneWithCache is called
	wg sync.WaitGroup
}

type newInformer func(ctx context.Context, client dynamic.ResourceInterface, fields map[string]informer.IndexedField, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, gvk schema.GroupVersionKind, db db.Client, shouldEncrypt bool, namespace bool, watchable bool, gcKeepCount int) (*informer.Informer, error)

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
	// Deprecated: events are not stored in memory using a fixed-length circular list
	GCInterval time.Duration
	// GCKeepCount is how many events to keep in memory
	GCKeepCount             int
	DBMetricsUpdateInterval time.Duration
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
	dbClient, dbPath, err := db.NewClient(ctx, nil, m, m, false)
	if err != nil {
		cancel()
		return nil, err
	}
	metrics.StartDatabaseMetricsLogger(ctx, dbPath, opts.DBMetricsUpdateInterval*time.Second)
	return &CacheFactory{
		ctx:    ctx,
		cancel: cancel,

		encryptAll: os.Getenv(EncryptAllEnvVar) == "true",
		dbClient:   dbClient,

		gcKeepCount: opts.GCKeepCount,

		newInformer: informer.NewInformer,
		informers:   map[schema.GroupVersionKind]*guardedInformer{},
	}, nil
}

func logCacheInitializationDuration(gvk schema.GroupVersionKind) func() {
	start := time.Now()
	log.Infof("CacheFor STARTS creating informer for %v", gvk)
	return func() {
		log.Infof("CacheFor IS DONE creating informer for %v (took %v)", gvk, time.Since(start))
	}
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
func (f *CacheFactory) CacheFor(ctx context.Context, fields map[string]informer.IndexedField, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool, watchable bool) (*Cache, error) {
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
			informer: nil,
			ctx:      giCtx,
			cancel:   giCancel,
		}
		f.informers[gvk] = gi
	}
	f.informersMutex.Unlock()

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if initialized, err := f.ensureInformerInitialized(gi, fields, externalUpdateInfo, selfUpdateInfo, transform, client, gvk, namespaced, watchable); err != nil {
			return nil, err
		} else if gi.informer == nil {
			// Special case for race condition: if Stop() is called while or immediately after this informer was initialized
			// Since we had to downgrade from Lock to RLock (two steps), gi.informer could have been reset in between.
			gi.mutex.RUnlock()
			continue
		} else if initialized {
			// Only log from the goroutine that initialized the informer
			defer logCacheInitializationDuration(gvk)()
		}
		break
	}
	defer gi.mutex.RUnlock()

	gvkCache, err := f.waitForCacheReady(ctx, gvk, gi)
	if err != nil {
		return nil, err
	}

	// from this point, it's responsibility of the caller to call DoneWithCache(gvkCache) once it's done
	gi.wg.Add(1)
	return gvkCache, nil
}

// ensureInformerInitialized handles the informer initialization, if needed, in which case, it returns true
// On success with gi.mutex.RLock() held
func (f *CacheFactory) ensureInformerInitialized(gi *guardedInformer, fields map[string]informer.IndexedField, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool, watchable bool) (bool, error) {
	// Fast path: read-lock and informer already initialized
	gi.mutex.RLock()
	if gi.informer != nil {
		return false, nil
	}
	gi.mutex.RUnlock()

	// Slow path: write lock and proceed to initialization.
	// But first, check if already initialized while waiting for write lock.
	gi.mutex.Lock()
	if gi.informer != nil {
		// Downgrade to RLock
		gi.mutex.Unlock()
		gi.mutex.RLock()
		return false, nil
	}

	if err := f.initializeInformerLocked(gi, fields, externalUpdateInfo, selfUpdateInfo, transform, client, gvk, namespaced, watchable); err != nil {
		gi.mutex.Unlock()
		// Error logging is already handled in initializeInformerLocked or caller can handle it
		return false, err
	}

	// Downgrade to RLock
	gi.mutex.Unlock()
	gi.mutex.RLock()
	return true, nil
}

func (f *CacheFactory) initializeInformerLocked(gi *guardedInformer, fields map[string]informer.IndexedField, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool, watchable bool) error {
	_, encryptResourceAlways := defaultEncryptedResourceTypes[gvk]
	shouldEncrypt := f.encryptAll || encryptResourceAlways
	// In non-test code this invokes pkg/sqlcache/informer/informer.go: NewInformer()
	// search for "func NewInformer(ctx"
	i, err := f.newInformer(gi.ctx, client, fields, externalUpdateInfo, selfUpdateInfo, transform, gvk, f.dbClient, shouldEncrypt, namespaced, watchable, f.gcKeepCount)
	if err != nil {
		log.Errorf("creating informer for %v: %v", gvk, err)
		return err
	}

	if err := i.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		if !watchable && errors.IsMethodNotSupported(err) {
			// expected, continue without logging
			return
		}
		cache.DefaultWatchErrorHandler(gi.ctx, r, err)
	}); err != nil {
		return fmt.Errorf("setting watch error handler: %w", err)
	}

	gi.informerDone = make(chan struct{})
	go func() {
		defer close(gi.informerDone)
		i.Run(gi.ctx.Done())
	}()
	gi.informer = i
	return nil
}

// waitForCacheReady will block until the informer has synced, or any of the context is canceled (the one from the request or the informer's own context)
func (f *CacheFactory) waitForCacheReady(ctx context.Context, gvk schema.GroupVersionKind, gi *guardedInformer) (*Cache, error) {
	// We don't want to get stuck in WaitForCachesSync if the request from
	// the client has been canceled.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
		case <-gi.ctx.Done():
			cancel()
		}
	}()

	if !cache.WaitForCacheSync(ctx.Done(), gi.informer.HasSynced) {
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

	cache.gi.wg.Done()
}

// Stop cancels ctx which stops any running informers, assigns a new ctx, resets the GVK-informer cache, and resets
// the database connection which wipes any current sqlite database at the default location.
func (f *CacheFactory) Stop(gvk schema.GroupVersionKind) error {
	if f.dbClient == nil {
		// nothing to reset
		return nil
	}

	f.informersMutex.Lock()
	gi, ok := f.informers[gvk]
	f.informersMutex.Unlock()
	if !ok {
		return nil
	}

	// Stop any ongoing WaitForCacheSync or Cache users, which will eventually call DoneWithCache as a result
	gi.cancel()

	// Prevent any new operations in this guarded informer
	gi.mutex.Lock()
	defer gi.mutex.Unlock()

	// Wait for all remaining DoneWithCache
	gi.wg.Wait()

	// Wait for informer to finish
	if gi.informerDone != nil {
		<-gi.informerDone
	}

	// Prepare for the next run
	gi.ctx, gi.cancel = context.WithCancel(f.ctx)

	if inf := gi.informer; inf != nil {
		// Discard gi.informer regardless of the result of dropping, as it's not retried anyway
		gi.informer = nil

		// DropAll needs its own context because the context from the informer
		// is canceled
		if err := inf.DropAll(f.ctx); err != nil {
			log.Errorf("error running informer.DropAll for %v: %v", gvk, err)
			return fmt.Errorf("dropall %q: %w", gvk, err)
		}
	}
	return nil
}

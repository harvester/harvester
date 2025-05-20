/*
Package factory provides a cache factory for the sql-based cache.
*/
package factory

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rancher/lasso/pkg/log"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/encryption"
	"github.com/rancher/steve/pkg/sqlcache/informer"
	sqlStore "github.com/rancher/steve/pkg/sqlcache/store"
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
	dbClient   DBClient
	stopCh     chan struct{}
	mutex      sync.RWMutex
	encryptAll bool

	newInformer newInformer

	informers      map[schema.GroupVersionKind]*guardedInformer
	informersMutex sync.Mutex
}

type guardedInformer struct {
	informer *informer.Informer
	mutex    *sync.Mutex
}

type newInformer func(client dynamic.ResourceInterface, fields [][]string, transform cache.TransformFunc, gvk schema.GroupVersionKind, db sqlStore.DBClient, shouldEncrypt bool, namespace bool) (*informer.Informer, error)

type DBClient interface {
	informer.DBClient
	sqlStore.DBClient
	connector
}

type Cache struct {
	informer.ByOptionsLister
}

type connector interface {
	NewConnection() error
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

// NewCacheFactory returns an informer factory instance
// This is currently called from steve via initial calls to `s.cacheFactory.CacheFor(...)`
func NewCacheFactory() (*CacheFactory, error) {
	m, err := encryption.NewManager()
	if err != nil {
		return nil, err
	}
	dbClient, err := db.NewClient(nil, m, m)
	if err != nil {
		return nil, err
	}
	return &CacheFactory{
		wg:          wait.Group{},
		stopCh:      make(chan struct{}),
		encryptAll:  os.Getenv(EncryptAllEnvVar) == "true",
		dbClient:    dbClient,
		newInformer: informer.NewInformer,
		informers:   map[schema.GroupVersionKind]*guardedInformer{},
	}, nil
}

// CacheFor returns an informer for given GVK, using sql store indexed with fields, using the specified client. For virtual fields, they must be added by the transform function
// and specified by fields to be used for later fields.
func (f *CacheFactory) CacheFor(fields [][]string, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool, watchable bool) (Cache, error) {
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
		i, err := f.newInformer(client, fields, transform, gvk, f.dbClient, shouldEncrypt, namespaced)
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
	err := f.dbClient.NewConnection()
	if err != nil {
		return err
	}

	return nil
}

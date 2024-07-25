/*
Package factory provides an cache factory for the sql-based cache.
*/
package factory

import (
	"fmt"
	"os"
	"sync"

	"github.com/rancher/lasso/pkg/cache/sql/informer"

	"github.com/rancher/lasso/pkg/cache/sql/attachdriver"
	"github.com/rancher/lasso/pkg/cache/sql/db"
	"github.com/rancher/lasso/pkg/cache/sql/encryption"
	sqlStore "github.com/rancher/lasso/pkg/cache/sql/store"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

// CacheFactory builds Informer instances and keeps a cache of instances it created
type CacheFactory struct {
	informerCreateLock sync.RWMutex
	wg                 wait.Group
	dbClient           DBClient
	stopCh             chan struct{}
	encryptAll         bool

	newInformer newInformer

	cache map[schema.GroupVersionKind]*informer.Informer
}

type newInformer func(client dynamic.ResourceInterface, fields [][]string, gvk schema.GroupVersionKind, db sqlStore.DBClient, shouldEncrypt bool, namespace bool) (*informer.Informer, error)

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
}

const (
	EncryptAllEnvVar = "CATTLE_ENCRYPT_CACHE_ALL"
)

func init() {
	indexedFieldDBPath := db.OnDiskInformerIndexedFieldDBPath
	if os.Getenv(EncryptAllEnvVar) == "true" {
		indexedFieldDBPath = ":memory:"
	}
	attachdriver.Register("file:" + indexedFieldDBPath + "?cache=shared")
}

// NewCacheFactory returns an informer factory instance
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
		cache:       map[schema.GroupVersionKind]*informer.Informer{},
		encryptAll:  os.Getenv(EncryptAllEnvVar) == "true",
		dbClient:    dbClient,
		newInformer: informer.NewInformer,
	}, nil
}

// CacheFor returns an informer for given GVK, using sql store indexed with fields, using the specified client
func (f *CacheFactory) CacheFor(fields [][]string, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, namespaced bool) (Cache, error) {
	result, ok := f.getCacheIfExists(gvk)
	if ok {
		return Cache{ByOptionsLister: result}, nil
	}

	f.informerCreateLock.Lock()
	defer f.informerCreateLock.Unlock()

	_, encryptResourceAlways := defaultEncryptedResourceTypes[gvk]
	shouldEncrypt := f.encryptAll || encryptResourceAlways
	i, err := f.newInformer(client, fields, gvk, f.dbClient, shouldEncrypt, namespaced)
	if err != nil {
		return Cache{}, err
	}

	if f.cache == nil {
		f.cache = make(map[schema.GroupVersionKind]*informer.Informer)
	}
	f.cache[gvk] = i
	f.wg.StartWithChannel(f.stopCh, i.Run)
	if !cache.WaitForCacheSync(f.stopCh, i.HasSynced) {
		return Cache{}, fmt.Errorf("failed to sync SQLite Informer cache for GVK %v", gvk)
	}

	return Cache{ByOptionsLister: i}, nil
}

func (f *CacheFactory) getCacheIfExists(gvk schema.GroupVersionKind) (*informer.Informer, bool) {
	f.informerCreateLock.RLock()
	defer f.informerCreateLock.RUnlock()

	result, ok := f.cache[gvk]
	if ok {
		return result, true
	}
	return nil, false
}

// Reset closes the stopCh which stops any running informers, assigns a new stopCh, resets the GVK-informer cache, and resets
// the database connection which wipes any current sqlite database at the default location.
func (f *CacheFactory) Reset() error {
	if f.dbClient == nil {
		// nothing to reset
		return nil
	}

	f.informerCreateLock.Lock()
	defer f.informerCreateLock.Unlock()

	close(f.stopCh)
	f.stopCh = make(chan struct{})
	f.cache = make(map[schema.GroupVersionKind]*informer.Informer)
	err := f.dbClient.NewConnection()
	if err != nil {
		return err
	}

	return nil
}

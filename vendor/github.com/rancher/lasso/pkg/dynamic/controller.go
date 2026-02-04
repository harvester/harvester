package dynamic

import (
	"context"
	"fmt"
	"sync"
	"time"

	lcache "github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/lasso/pkg/log"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/cache"
)

type Handler func(obj runtime.Object) (runtime.Object, error)
type GVKMatcher func(gvk schema.GroupVersionKind) bool

type handlerEntry struct {
	matcher GVKMatcher
	handler Handler
}

type watcher struct {
	ctx        context.Context
	cancel     func()
	informer   cache.SharedIndexInformer
	controller controller.Controller
	gvk        schema.GroupVersionKind
}

type Controller struct {
	sync.RWMutex

	ctx context.Context

	discovery     discovery.DiscoveryInterface
	cacheFactory  lcache.SharedCacheFactory
	clientFactory client.SharedClientFactory
	watchers      map[schema.GroupVersionKind]*watcher
	indexers      []indexerEntry

	handlers lcache.CancelCollection
	handler  controller.SharedHandler
}

func (c *Controller) Register(ctx context.Context, factory controller.SharedControllerFactory) error {
	c.ctx = ctx
	c.cacheFactory = factory.SharedCacheFactory()
	c.clientFactory = factory.SharedCacheFactory().SharedClientFactory()
	return watchGVKS(ctx, c.discovery, factory, c.OnGVKs)
}

func New(discovery discovery.DiscoveryInterface) *Controller {
	c := &Controller{
		discovery: discovery,
		watchers:  map[schema.GroupVersionKind]*watcher{},
	}
	return c
}

func (c *Controller) validGVK(gvk schema.GroupVersionKind) bool {
	for _, obj := range c.handlers.List() {
		handler := obj.(*handlerEntry)
		if handler.matcher(gvk) {
			return true
		}
	}
	// these are caches that were started on demand
	for _, obj := range c.watchers {
		if gvk == obj.gvk {
			return true
		}
	}
	return false
}

func (c *Controller) AddIndexer(name string, matcher GVKMatcher, indexer func(obj runtime.Object) ([]string, error)) {
	c.Lock()
	defer c.Unlock()
	c.indexers = append(c.indexers, indexerEntry{
		matcher: matcher,
		name:    name,
		indexer: indexer,
	})

	for gvk, watcher := range c.watchers {
		if matcher(gvk) {
			watcher.informer.AddIndexers(cache.Indexers{
				name: func(obj interface{}) ([]string, error) {
					return indexer(obj.(runtime.Object))
				},
			})
		}
	}
}

func (c *Controller) OnChange(ctx context.Context, name string, matcher GVKMatcher, handler Handler) {
	c.handler.Register(ctx, name, wrap(matcher, handler))
	c.handlers.Add(ctx, &handlerEntry{
		matcher: matcher,
		handler: handler,
	})
}

func (c *Controller) getCache(ctx context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, bool, error) {
	if c.cacheFactory.WaitForCacheSync(ctx)[gvk] {
		cache, err := c.cacheFactory.ForKind(gvk)
		return cache, true, err
	}

	client, err := c.clientFactory.ForKind(gvk)
	if err != nil {
		return nil, false, err
	}

	obj, objList, err := c.clientFactory.NewObjects(gvk)
	if err != nil {
		return nil, false, err
	}

	return lcache.NewCache(obj, objList, client, nil), false, nil
}

func (c *Controller) OnGVKs(gvkList []schema.GroupVersionKind) error {
	c.Lock()
	defer c.Unlock()
	return c.setGVKs(gvkList, schema.GroupVersionKind{})
}

func (c *Controller) setGVKs(gvkList []schema.GroupVersionKind, additionalValidGVK schema.GroupVersionKind) error {
	var (
		gvks               = map[schema.GroupVersionKind]bool{}
		toWait             []*watcher
		errs               []error
		timeoutCtx, cancel = context.WithTimeout(c.ctx, 15*time.Minute)
	)
	defer cancel()

outer:
	for _, gvk := range gvkList {
		if !c.validGVK(gvk) && gvk != additionalValidGVK {
			continue
		}

		gvks[gvk] = true

		if c.watchers[gvk] != nil {
			continue
		}

		informer, shared, err := c.getCache(timeoutCtx, gvk)
		if err != nil {
			errs = append(errs, err)
			log.Errorf("Failed to get shared cache for %v: %v", gvk, err)
			delete(gvks, gvk)
			continue
		}

		for _, indexer := range c.indexers {
			if indexer.matcher(gvk) {
				err := informer.AddIndexers(cache.Indexers{
					indexer.name: func(obj interface{}) ([]string, error) {
						return indexer.indexer(obj.(runtime.Object))
					},
				})
				if err != nil {
					errs = append(errs, err)
					log.Errorf("failed to add indexer %s to gvk %s: %v", indexer.name, gvk, err)
					delete(gvks, gvk)
					continue outer
				}
			}
		}

		controller := controller.New(gvk.String(), informer, func(ctx context.Context) error {
			return nil
		}, &c.handler, nil)

		ctx, cancel := context.WithCancel(c.ctx)
		w := &watcher{
			ctx:        ctx,
			cancel:     cancel,
			gvk:        gvk,
			informer:   informer,
			controller: controller,
		}
		c.watchers[gvk] = w
		toWait = append(toWait, w)

		if !shared {
			log.Infof("Watching metadata for %s", w.gvk)
			go w.informer.Run(w.ctx.Done())
		}
	}

	for gvk, w := range c.watchers {
		if !gvks[gvk] {
			log.Infof("Stopping metadata watch on %s", gvk)
			w.cancel()
			delete(c.watchers, gvk)
		}
	}

	for _, w := range toWait {
		if !cache.WaitForCacheSync(timeoutCtx.Done(), w.informer.HasSynced) {
			errs = append(errs, fmt.Errorf("failed to sync cache for %v", w.gvk))
			log.Errorf("failed to sync cache for %v", w.gvk)
			w.cancel()
			delete(c.watchers, w.gvk)
		}
	}

	for _, w := range toWait {
		if err := w.controller.Start(w.ctx, 5); err != nil {
			errs = append(errs, err)
			log.Errorf("failed to start controller for %v: %v", w.gvk, err)
			w.cancel()
			delete(c.watchers, w.gvk)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func (c *Controller) GetCache(_ context.Context, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, bool, error) {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return nil, false, err
	}
	return w.informer, w.informer.HasSynced(), nil
}

func (c *Controller) getWatcherForGVK(gvk schema.GroupVersionKind) (*watcher, error) {
	c.RLock()
	w, ok := c.watchers[gvk]
	if ok {
		c.RUnlock()
		return w, nil
	}
	c.RUnlock()

	// check type exists on the server
	_, err := c.clientFactory.ForKind(gvk)
	if err != nil {
		return nil, err
	}

	c.Lock()
	defer c.Unlock()

	gvks := make([]schema.GroupVersionKind, 0, len(c.watchers)+1)
	for gvk := range c.watchers {
		gvks = append(gvks, gvk)
	}
	gvks = append(gvks, gvk)

	if err := c.setGVKs(gvks, gvk); err != nil {
		return nil, err
	}

	w, ok = c.watchers[gvk]
	if !ok {
		return nil, fmt.Errorf("failed to load informer for %v", gvk)
	}
	return w, nil
}

func (c *Controller) Get(gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, error) {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return nil, err
	}

	var key string
	if namespace == "" {
		key = name
	} else {
		key = namespace + "/" + name
	}

	obj, ok, err := w.informer.GetStore().GetByKey(key)
	if err != nil {
		return nil, err
	}
	if rObj, isRObj := obj.(runtime.Object); isRObj && ok {
		return rObj, nil
	}
	return nil, errors.NewNotFound(schema.GroupResource{
		Group:    gvk.Group,
		Resource: gvk.Kind,
	}, key)
}

func (c *Controller) UpdateStatus(obj runtime.Object) (runtime.Object, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	client, err := c.clientFactory.ForKind(gvk)
	if err != nil {
		return nil, err
	}

	result := &unstructured.Unstructured{}
	return result, client.UpdateStatus(c.ctx, meta.GetNamespace(), obj, result, v1.UpdateOptions{})
}

func (c *Controller) Update(obj runtime.Object) (runtime.Object, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	client, err := c.clientFactory.ForKind(gvk)
	if err != nil {
		return nil, err
	}

	result := &unstructured.Unstructured{}
	return result, client.Update(c.ctx, meta.GetNamespace(), obj, result, v1.UpdateOptions{})
}

func (c *Controller) Enqueue(gvk schema.GroupVersionKind, namespace, name string) error {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return err
	}

	w.controller.Enqueue(namespace, name)
	return nil
}

func (c *Controller) EnqueueAfter(gvk schema.GroupVersionKind, namespace, name string, delay time.Duration) error {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return err
	}

	w.controller.EnqueueAfter(namespace, name, delay)
	return nil
}

func (c *Controller) GetByIndex(gvk schema.GroupVersionKind, indexName, key string) ([]runtime.Object, error) {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return nil, err
	}

	objs, err := w.informer.GetIndexer().ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}

	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(runtime.Object))
	}
	return result, nil
}

func (c *Controller) List(gvk schema.GroupVersionKind, namespace string, selector labels.Selector) ([]runtime.Object, error) {
	w, err := c.getWatcherForGVK(gvk)
	if err != nil {
		return nil, err
	}

	var result []runtime.Object
	err = cache.ListAllByNamespace(w.informer.GetIndexer(), namespace, selector, func(obj interface{}) {
		result = append(result, obj.(runtime.Object))
	})

	return result, err
}

func wrap(matcher GVKMatcher, handler Handler) controller.SharedControllerHandler {
	return controller.SharedControllerHandlerFunc(func(key string, obj runtime.Object) (runtime.Object, error) {
		if obj == nil {
			return nil, nil
		}
		gvk := obj.GetObjectKind().GroupVersionKind()
		if matcher(gvk) {
			return handler(obj)
		}
		return obj, nil
	})
}

func FromKeyHandler(handler func(string, runtime.Object) (runtime.Object, error)) Handler {
	return func(obj runtime.Object) (runtime.Object, error) {
		meta, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		if meta.GetNamespace() == "" {
			return handler(meta.GetName(), obj)
		}
		return handler(meta.GetNamespace()+"/"+meta.GetName(), obj)
	}
}

type indexerEntry struct {
	matcher GVKMatcher
	name    string
	indexer func(runtime.Object) ([]string, error)
}

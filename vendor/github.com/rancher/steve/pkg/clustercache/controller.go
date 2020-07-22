package clustercache

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/wrangler/pkg/merr"
	"github.com/rancher/wrangler/pkg/summary/client"
	"github.com/rancher/wrangler/pkg/summary/informer"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	schema2 "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Handler func(gvr schema2.GroupVersionResource, key string, obj runtime.Object) error
type ChangeHandler func(gvr schema2.GroupVersionResource, key string, obj, oldObj runtime.Object) error

type ClusterCache interface {
	List(gvr schema2.GroupVersionResource) []interface{}
	OnAdd(ctx context.Context, handler Handler)
	OnRemove(ctx context.Context, handler Handler)
	OnChange(ctx context.Context, handler ChangeHandler)
	OnSchemas(schemas *schema.Collection) error
}

type event struct {
	add    bool
	gvr    schema2.GroupVersionResource
	obj    runtime.Object
	oldObj runtime.Object
}

type watcher struct {
	ctx      context.Context
	cancel   func()
	informer cache.SharedIndexInformer
	gvk      schema2.GroupVersionKind
	gvr      schema2.GroupVersionResource
}

type clusterCache struct {
	sync.RWMutex

	ctx           context.Context
	summaryClient client.Interface
	watchers      map[schema2.GroupVersionResource]*watcher
	workqueue     workqueue.DelayingInterface

	addHandlers    cancelCollection
	removeHandlers cancelCollection
	changeHandlers cancelCollection
}

func NewClusterCache(ctx context.Context, dynamicClient dynamic.Interface) ClusterCache {
	c := &clusterCache{
		ctx:           ctx,
		summaryClient: client.NewForDynamicClient(dynamicClient),
		watchers:      map[schema2.GroupVersionResource]*watcher{},
		workqueue:     workqueue.NewNamedDelayingQueue("cluster-cache"),
	}
	go c.start()
	return c
}

func validSchema(schema *types.APISchema) bool {
	canList := false
	canWatch := false
	for _, verb := range attributes.Verbs(schema) {
		switch verb {
		case "list":
			canList = true
		case "watch":
			canWatch = true
		}
	}

	if !canList || !canWatch {
		return false
	}

	return true
}

func (h *clusterCache) addResourceEventHandler(gvr schema2.GroupVersionResource, informer cache.SharedIndexInformer) {
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if rObj, ok := obj.(runtime.Object); ok {
				h.workqueue.Add(event{
					add: true,
					obj: rObj,
					gvr: gvr,
				})
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if rObj, ok := newObj.(runtime.Object); ok {
				if rOldObj, ok := oldObj.(runtime.Object); ok {
					h.workqueue.Add(event{
						obj:    rObj,
						oldObj: rOldObj,
						gvr:    gvr,
					})
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			if rObj, ok := obj.(runtime.Object); ok {
				h.workqueue.Add(event{
					obj: rObj,
					gvr: gvr,
				})
			}
		},
	})
}

func (h *clusterCache) OnSchemas(schemas *schema.Collection) error {
	h.Lock()
	defer h.Unlock()

	var (
		gvrs   = map[schema2.GroupVersionResource]bool{}
		toWait []*watcher
	)

	for _, id := range schemas.IDs() {
		schema := schemas.Schema(id)
		if !validSchema(schema) {
			continue
		}

		gvr := attributes.GVR(schema)
		gvk := attributes.GVK(schema)
		gvrs[gvr] = true

		if h.watchers[gvr] != nil {
			continue
		}

		summaryInformer := informer.NewFilteredSummaryInformer(h.summaryClient, gvr, metav1.NamespaceAll, 2*time.Hour,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, nil)
		ctx, cancel := context.WithCancel(h.ctx)
		w := &watcher{
			ctx:      ctx,
			cancel:   cancel,
			gvk:      gvk,
			gvr:      gvr,
			informer: summaryInformer.Informer(),
		}
		h.watchers[gvr] = w
		toWait = append(toWait, w)

		logrus.Infof("Watching metadata for %s", w.gvk)
		h.addResourceEventHandler(w.gvr, w.informer)
		go w.informer.Run(w.ctx.Done())
	}

	for gvr, w := range h.watchers {
		if !gvrs[gvr] {
			logrus.Infof("Stopping metadata watch on %s", gvr)
			w.cancel()
			delete(h.watchers, gvr)
		}
	}

	for _, w := range toWait {
		ctx, cancel := context.WithTimeout(w.ctx, 15*time.Minute)
		if !cache.WaitForCacheSync(ctx.Done(), w.informer.HasSynced) {
			logrus.Errorf("failed to sync cache for %v", w.gvk)
			cancel()
			w.cancel()
			delete(h.watchers, w.gvr)
		}
		cancel()
	}

	return nil
}

func (h *clusterCache) List(gvr schema2.GroupVersionResource) []interface{} {
	h.RLock()
	defer h.RUnlock()

	w, ok := h.watchers[gvr]
	if !ok {
		return nil
	}

	return w.informer.GetStore().List()
}

func (h *clusterCache) start() {
	defer h.workqueue.ShutDown()
	for {
		eventObj, ok := h.workqueue.Get()
		if ok {
			break
		}

		event := eventObj.(event)
		h.RLock()
		w := h.watchers[event.gvr]
		h.RUnlock()
		if w == nil {
			h.workqueue.Done(eventObj)
			continue
		}

		key := toKey(event.obj)
		if event.oldObj != nil {
			_, err := callAll(h.changeHandlers.List(), event.gvr, key, event.obj, event.oldObj)
			if err != nil {
				logrus.Errorf("failed to handle add event: %v", err)
			}
		} else if event.add {
			_, err := callAll(h.addHandlers.List(), event.gvr, key, event.obj, nil)
			if err != nil {
				logrus.Errorf("failed to handle add event: %v", err)
			}
		} else {
			_, err := callAll(h.removeHandlers.List(), event.gvr, key, event.obj, nil)
			if err != nil {
				logrus.Errorf("failed to handle remove event: %v", err)
			}
		}
		h.workqueue.Done(eventObj)
	}
}

func toKey(obj runtime.Object) string {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return ""
	}
	ns := meta.GetNamespace()
	if ns == "" {
		return meta.GetName()
	}
	return ns + "/" + meta.GetName()
}

func (h *clusterCache) OnAdd(ctx context.Context, handler Handler) {
	h.addHandlers.Add(ctx, handler)
}

func (h *clusterCache) OnRemove(ctx context.Context, handler Handler) {
	h.removeHandlers.Add(ctx, handler)
}

func (h *clusterCache) OnChange(ctx context.Context, handler ChangeHandler) {
	h.changeHandlers.Add(ctx, handler)
}

func callAll(handlers []interface{}, gvr schema2.GroupVersionResource, key string, obj, oldObj runtime.Object) (runtime.Object, error) {
	var errs []error
	for _, handler := range handlers {
		if f, ok := handler.(Handler); ok {
			if err := f(gvr, key, obj); err != nil {
				errs = append(errs, err)
			}
		}
		if f, ok := handler.(ChangeHandler); ok {
			if err := f(gvr, key, obj, oldObj); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return obj, merr.NewErrors(errs...)
}

package relatedresource

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type Key struct {
	Namespace string
	Name      string
}

func NewKey(namespace, name string) Key {
	return Key{
		Namespace: namespace,
		Name:      name,
	}
}

func FromString(key string) Key {
	return NewKey(kv.RSplit(key, "/"))
}

type ControllerWrapper interface {
	Informer() cache.SharedIndexInformer
	AddGenericHandler(ctx context.Context, name string, handler generic.Handler)
}

type ClusterScopedEnqueuer interface {
	Enqueue(name string)
}

type Enqueuer interface {
	Enqueue(namespace, name string)
}

type Resolver func(namespace, name string, obj runtime.Object) ([]Key, error)

func WatchClusterScoped(ctx context.Context, name string, resolve Resolver, enq ClusterScopedEnqueuer, watching ...ControllerWrapper) {
	Watch(ctx, name, resolve, &wrapper{ClusterScopedEnqueuer: enq}, watching...)
}

func Watch(ctx context.Context, name string, resolve Resolver, enq Enqueuer, watching ...ControllerWrapper) {
	for _, c := range watching {
		watch(ctx, name, enq, resolve, c)
	}
}

func watch(ctx context.Context, name string, enq Enqueuer, resolve Resolver, controller ControllerWrapper) {
	runResolve := func(ns, name string, obj runtime.Object) error {
		keys, err := resolve(ns, name, obj)
		if err != nil {
			return err
		}

		for _, key := range keys {
			if key.Name != "" {
				enq.Enqueue(key.Namespace, key.Name)
			}
		}

		return nil
	}

	addResourceEventHandler(ctx, controller.Informer(), cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			ro, ok := obj.(runtime.Object)
			if !ok {
				return
			}

			meta, err := meta.Accessor(ro)
			if err != nil {
				return
			}

			go func() {
				time.Sleep(time.Second)
				runResolve(meta.GetNamespace(), meta.GetName(), ro)
			}()
		},
	})

	controller.AddGenericHandler(ctx, name, func(key string, obj runtime.Object) (runtime.Object, error) {
		ns, name := kv.RSplit(key, "/")
		return obj, runResolve(ns, name, obj)
	})
}

type wrapper struct {
	ClusterScopedEnqueuer
}

func (w *wrapper) Enqueue(namespace, name string) {
	w.ClusterScopedEnqueuer.Enqueue(name)
}

// informerRegisterer is a subset of the cache.SharedIndexInformer, so it's easier to replace in tests
type informerRegisterer interface {
	AddEventHandler(funcs cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	RemoveEventHandler(cache.ResourceEventHandlerRegistration) error
}

func addResourceEventHandler(ctx context.Context, informer informerRegisterer, handler cache.ResourceEventHandler) {
	handlerReg, err := informer.AddEventHandler(handler)
	if err != nil {
		logrus.WithError(err).Error("failed to add ResourceEventHandler")
		return
	}
	go func() {
		<-ctx.Done()
		if err := informer.RemoveEventHandler(handlerReg); err != nil {
			logrus.WithError(err).Warn("failed to remove ResourceEventHandler")
		}
	}()
}

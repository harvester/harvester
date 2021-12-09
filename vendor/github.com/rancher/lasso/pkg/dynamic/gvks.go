package dynamic

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rancher/lasso/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"
)

type gvksCallback func([]schema.GroupVersionKind) error

type gvkWatcher struct {
	sync.Mutex

	toSync   int32
	client   discovery.DiscoveryInterface
	callback gvksCallback
}

func watchGVKS(ctx context.Context,
	discovery discovery.DiscoveryInterface,
	factory controller.SharedControllerFactory,
	callback gvksCallback) error {
	h := &gvkWatcher{
		client:   discovery,
		callback: callback,
	}

	crdController, err := factory.ForKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	if err != nil {
		return err
	}

	apiServiceController, err := factory.ForKind(schema.GroupVersionKind{
		Group:   "apiregistration.k8s.io",
		Version: "v1",
		Kind:    "APIService",
	})
	if err != nil {
		return err
	}

	crdController.RegisterHandler(ctx, "dynamic-types", controller.SharedControllerHandlerFunc(h.onTypeChange))
	apiServiceController.RegisterHandler(ctx, "dynamic-types", controller.SharedControllerHandlerFunc(h.onTypeChange))
	return nil
}

func (g *gvkWatcher) onTypeChange(_ string, obj runtime.Object) (runtime.Object, error) {
	g.queueRefresh()
	return obj, nil
}

func (g *gvkWatcher) queueRefresh() {
	atomic.StoreInt32(&g.toSync, 1)

	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := g.refreshAll(); err != nil {
			klog.Errorf("failed to sync schemas: %v", err)
			atomic.StoreInt32(&g.toSync, 1)
		}
	}()
}

func (g *gvkWatcher) getGVKs() (result []schema.GroupVersionKind, _ error) {
	_, resources, err := g.client.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		for _, apiResource := range resource.APIResources {
			result = append(result, schema.FromAPIVersionAndKind(resource.GroupVersion, apiResource.Kind))
		}
	}
	return result, nil
}

func (g *gvkWatcher) refreshAll() error {
	g.Lock()
	defer g.Unlock()

	if !g.needToSync() {
		return nil
	}

	gvks, err := g.getGVKs()
	if err != nil {
		return err
	}

	return g.callback(gvks)
}

func (g *gvkWatcher) needToSync() bool {
	old := atomic.SwapInt32(&g.toSync, 0)
	return old == 1
}

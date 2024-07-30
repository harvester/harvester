// Package generic provides generic types and implementations for Controllers, Clients, and Caches.
package generic

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/rancher/lasso/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// ErrSkip notifies the caller to skip this error.
var ErrSkip = controller.ErrIgnore

// ControllerMeta holds meta information shared by all controllers.
type ControllerMeta interface {
	// Informer returns the SharedIndexInformer used by this controller.
	Informer() cache.SharedIndexInformer

	// GroupVersionKind returns the GVK used to create this Controller.
	GroupVersionKind() schema.GroupVersionKind

	// AddGenericHandler adds a generic handler that runs when a resource changes.
	AddGenericHandler(ctx context.Context, name string, handler Handler)

	// AddGenericHandler adds a generic handler that runs when a resource is removed.
	AddGenericRemoveHandler(ctx context.Context, name string, handler Handler)

	// Updater returns a update function that will attempt to perform an update for a specific resource type.
	Updater() Updater
}

// RuntimeMetaObject is an interface for a K8s Object to be used with a specific controller.
type RuntimeMetaObject interface {
	comparable
	runtime.Object
	metav1.Object
}

// ControllerInterface interface for managing K8s Objects.
type ControllerInterface[T RuntimeMetaObject, TList runtime.Object] interface {
	ControllerMeta
	ClientInterface[T, TList]

	// OnChange runs the given object handler when the controller detects a resource was changed.
	OnChange(ctx context.Context, name string, sync ObjectHandler[T])

	// OnRemove runs the given object handler when the controller detects a resource was changed.
	OnRemove(ctx context.Context, name string, sync ObjectHandler[T])

	// Enqueue adds the resource with the given name in the provided namespace to the worker queue of the controller.
	Enqueue(namespace, name string)

	// EnqueueAfter runs Enqueue after the provided duration.
	EnqueueAfter(namespace, name string, duration time.Duration)

	// Cache returns a cache for the resource type T.
	Cache() CacheInterface[T]
}

// NonNamespacedControllerInterface interface for managing non namespaced K8s Objects.
type NonNamespacedControllerInterface[T RuntimeMetaObject, TList runtime.Object] interface {
	ControllerMeta
	NonNamespacedClientInterface[T, TList]

	// OnChange runs the given object handler when the controller detects a resource was changed.
	OnChange(ctx context.Context, name string, sync ObjectHandler[T])

	// OnRemove runs the given object handler when the controller detects a resource was changed.
	OnRemove(ctx context.Context, name string, sync ObjectHandler[T])

	// Enqueue adds the resource with the given name to the worker queue of the controller.
	Enqueue(name string)

	// EnqueueAfter runs Enqueue after the provided duration.
	EnqueueAfter(name string, duration time.Duration)

	// Cache returns a cache for the resource type T.
	Cache() NonNamespacedCacheInterface[T]
}

// ClientInterface is an interface to performs CRUD like operations on an Objects.
type ClientInterface[T RuntimeMetaObject, TList runtime.Object] interface {
	// Create creates a new object and return the newly created Object or an error.
	Create(T) (T, error)

	// Update updates the object and return the newly updated Object or an error.
	Update(T) (T, error)

	// UpdateStatus updates the Status field of a the object and return the newly updated Object or an error.
	// Will always return an error if the object does not have a status field.
	UpdateStatus(T) (T, error)

	// Delete deletes the Object in the given name and namespace.
	Delete(namespace, name string, options *metav1.DeleteOptions) error

	// Get will attempt to retrieve the resource with the given name in the given namespace.
	Get(namespace, name string, options metav1.GetOptions) (T, error)

	// List will attempt to find resources in the given namespace.
	List(namespace string, opts metav1.ListOptions) (TList, error)

	// Watch will start watching resources in the given namespace.
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)

	// Patch will patch the resource with the matching name in the matching namespace.
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result T, err error)

	// WithImpersonation returns a new copy of the client that uses impersonation.
	WithImpersonation(impersonate rest.ImpersonationConfig) (ClientInterface[T, TList], error)
}

// NonNamespacedClientInterface is an interface to performs CRUD like operations on nonNamespaced Objects.
type NonNamespacedClientInterface[T RuntimeMetaObject, TList runtime.Object] interface {
	// Create creates a new object and return the newly created Object or an error.
	Create(T) (T, error)

	// Update updates the object and return the newly updated Object or an error.
	Update(T) (T, error)

	// UpdateStatus updates the Status field of a the object and return the newly updated Object or an error.
	// Will always return an error if the object does not have a status field.
	UpdateStatus(T) (T, error)

	// Delete deletes the Object in the given name.
	Delete(name string, options *metav1.DeleteOptions) error

	// Get will attempt to retrieve the resource with the specified name.
	Get(name string, options metav1.GetOptions) (T, error)

	// List will attempt to find multiple resources.
	List(opts metav1.ListOptions) (TList, error)

	// Watch will start watching resources.
	Watch(opts metav1.ListOptions) (watch.Interface, error)

	// Patch will patch the resource with the matching name.
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result T, err error)

	// WithImpersonation returns a new copy of the client that uses impersonation.
	WithImpersonation(impersonate rest.ImpersonationConfig) (NonNamespacedClientInterface[T, TList], error)
}

// ObjectHandler performs operations on the given runtime.Object and returns the new runtime.Object or an error
type Handler func(key string, obj runtime.Object) (runtime.Object, error)

// ObjectHandler performs operations on the given object and returns the new object or an error
type ObjectHandler[T runtime.Object] func(string, T) (T, error)

// Indexer computes a set of indexed values for the provided object.
type Indexer[T runtime.Object] func(obj T) ([]string, error)

// FromObjectHandlerToHandler converts an ObjecHandler to a Handler.
func FromObjectHandlerToHandler[T RuntimeMetaObject](sync ObjectHandler[T]) Handler {
	return func(key string, obj runtime.Object) (runtime.Object, error) {
		var nilObj, retObj T
		var err error
		if obj == nil {
			retObj, err = sync(key, nilObj)
		} else {
			retObj, err = sync(key, obj.(T))
		}
		if retObj == nilObj {
			return nil, err
		}
		return retObj, err
	}
}

// Controller is used to manage objects of type T.
type Controller[T RuntimeMetaObject, TList runtime.Object] struct {
	controller controller.SharedController
	embeddedClient
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
	objType       reflect.Type
	objListType   reflect.Type
}

// NonNamespacedController is a Controller for non namespaced resources. This controller provides similar function definitions as Controller except the namespace parameter is omitted.
type NonNamespacedController[T RuntimeMetaObject, TList runtime.Object] struct {
	*Controller[T, TList]
}

// NewController creates a new controller for the given Object type and ObjectList type.
func NewController[T RuntimeMetaObject, TList runtime.Object](gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) *Controller[T, TList] {
	sharedCtrl := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	var obj T
	objPtrType := reflect.TypeOf(obj)
	if objPtrType.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("Controller requires Object T to be a pointer not %v", objPtrType))
	}
	var objList TList
	objListPtrType := reflect.TypeOf(objList)
	if objListPtrType.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("Controller requires Object TList to be a pointer not %v", objListPtrType))
	}
	return &Controller[T, TList]{
		controller:     sharedCtrl,
		embeddedClient: sharedCtrl.Client(),
		gvk:            gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
		objType:     objPtrType.Elem(),
		objListType: objListPtrType.Elem(),
	}
}

// Updater creates a new Updater for the Object type T.
func (c *Controller[T, TList]) Updater() Updater {
	var nilObj T
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(T))
		if newObj == nilObj {
			return nil, err
		}
		return newObj, err
	}
}

// AddGenericHandler runs the given handler when the controller detects an object was changed.
func (c *Controller[T, TList]) AddGenericHandler(ctx context.Context, name string, handler Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

// AddGenericRemoveHandler runs the given handler when the controller detects an object was removed.
func (c *Controller[T, TList]) AddGenericRemoveHandler(ctx context.Context, name string, handler Handler) {
	c.AddGenericHandler(ctx, name, NewRemoveHandler(name, c.Updater(), handler))
}

// OnChange runs the given object handler when the controller detects a resource was changed.
func (c *Controller[T, TList]) OnChange(ctx context.Context, name string, sync ObjectHandler[T]) {
	c.AddGenericHandler(ctx, name, FromObjectHandlerToHandler(sync))
}

// OnRemove runs the given object handler when the controller detects a resource was changed.
func (c *Controller[T, TList]) OnRemove(ctx context.Context, name string, sync ObjectHandler[T]) {
	c.AddGenericHandler(ctx, name, NewRemoveHandler(name, c.Updater(), FromObjectHandlerToHandler(sync)))
}

// Enqueue adds the resource with the given name in the provided namespace to the worker queue of the controller.
func (c *Controller[T, TList]) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

// EnqueueAfter runs Enqueue after the provided duration.
func (c *Controller[T, TList]) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

// Informer returns the SharedIndexInformer used by this controller.
func (c *Controller[T, TList]) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

// GroupVersionKind returns the GVK used to create this Controller.
func (c *Controller[T, TList]) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

// Cache returns a cache for the objects T.
func (c *Controller[T, TList]) Cache() CacheInterface[T] {
	return &Cache[T]{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

// Create creates a new object and return the newly created Object or an error.
func (c *Controller[T, TList]) Create(obj T) (T, error) {
	result := reflect.New(c.objType).Interface().(T)
	return result, c.embeddedClient.Create(context.TODO(), obj.GetNamespace(), obj, result, metav1.CreateOptions{})
}

// Update updates the object and return the newly updated Object or an error.
func (c *Controller[T, TList]) Update(obj T) (T, error) {
	result := reflect.New(c.objType).Interface().(T)
	return result, c.embeddedClient.Update(context.TODO(), obj.GetNamespace(), obj, result, metav1.UpdateOptions{})
}

// UpdateStatus updates the Status field of a the object and return the newly updated Object or an error.
// Will always return an error if the object does not have a status field.
func (c *Controller[T, TList]) UpdateStatus(obj T) (T, error) {
	result := reflect.New(c.objType).Interface().(T)
	return result, c.embeddedClient.UpdateStatus(context.TODO(), obj.GetNamespace(), obj, result, metav1.UpdateOptions{})
}

// Delete deletes the Object in the given name and Namespace.
func (c *Controller[T, TList]) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.embeddedClient.Delete(context.TODO(), namespace, name, *options)
}

// Get gets returns the given resource with the given name in the provided namespace.
func (c *Controller[T, TList]) Get(namespace, name string, options metav1.GetOptions) (T, error) {
	result := reflect.New(c.objType).Interface().(T)
	return result, c.embeddedClient.Get(context.TODO(), namespace, name, result, options)
}

// List will attempt to find resources in the given namespace.
func (c *Controller[T, TList]) List(namespace string, opts metav1.ListOptions) (TList, error) {
	result := reflect.New(c.objListType).Interface().(TList)
	return result, c.embeddedClient.List(context.TODO(), namespace, result, opts)
}

// Watch will start watching resources in the given namespace.
func (c *Controller[T, TList]) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.embeddedClient.Watch(context.TODO(), namespace, opts)
}

// Patch will patch the resource with the matching name in the matching namespace.
func (c *Controller[T, TList]) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (T, error) {
	result := reflect.New(c.objType).Interface().(T)
	return result, c.embeddedClient.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

// WithImpersonation returns a new copy of the client that uses impersonation.
func (c *Controller[T, TList]) WithImpersonation(impersonate rest.ImpersonationConfig) (ClientInterface[T, TList], error) {
	newClient, err := c.embeddedClient.WithImpersonation(impersonate)
	if err != nil {
		return nil, fmt.Errorf("failed to make new client: %w", err)
	}

	// return a new controller with a new embeddedClient
	return &Controller[T, TList]{
		controller:     c.controller,
		embeddedClient: newClient,
		objType:        c.objType,
		objListType:    c.objListType,
		gvk:            c.gvk,
		groupResource:  c.groupResource,
	}, nil
}

// NewNonNamespacedController returns a Controller controller that is not namespaced.
// NonNamespacedController redefines specific functions to no longer accept the namespace parameter.
func NewNonNamespacedController[T RuntimeMetaObject, TList runtime.Object](gvk schema.GroupVersionKind, resource string,
	controller controller.SharedControllerFactory,
) *NonNamespacedController[T, TList] {
	ctrl := NewController[T, TList](gvk, resource, false, controller)
	return &NonNamespacedController[T, TList]{
		Controller: ctrl,
	}
}

// Enqueue calls Controller.Enqueue(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) Enqueue(name string) {
	c.Controller.Enqueue(metav1.NamespaceAll, name)
}

// EnqueueAfter calls Controller.EnqueueAfter(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) EnqueueAfter(name string, duration time.Duration) {
	c.Controller.EnqueueAfter(metav1.NamespaceAll, name, duration)
}

// Delete calls Controller.Delete(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) Delete(name string, options *metav1.DeleteOptions) error {
	return c.Controller.Delete(metav1.NamespaceAll, name, options)
}

// Get calls Controller.Get(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) Get(name string, options metav1.GetOptions) (T, error) {
	return c.Controller.Get(metav1.NamespaceAll, name, options)
}

// List calls Controller.List(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) List(opts metav1.ListOptions) (TList, error) {
	return c.Controller.List(metav1.NamespaceAll, opts)
}

// Watch calls Controller.Watch(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c.Controller.Watch(metav1.NamespaceAll, opts)
}

// Patch calls the Controller.Patch(...) with an empty namespace parameter.
func (c *NonNamespacedController[T, TList]) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (T, error) {
	return c.Controller.Patch(metav1.NamespaceAll, name, pt, data, subresources...)
}

// WithImpersonation returns a new copy of the client that uses impersonation.
func (c *NonNamespacedController[T, TList]) WithImpersonation(impersonate rest.ImpersonationConfig) (NonNamespacedClientInterface[T, TList], error) {
	newClient, err := c.Controller.WithImpersonation(impersonate)
	if err != nil {
		return nil, fmt.Errorf("failed to make new client: %w", err)
	}
	// get the underlying controller so we can wrap it in a NonNamespacedController
	newCtrl, ok := newClient.(*Controller[T, TList])
	if !ok {
		return nil, fmt.Errorf("failed to make new client from: %T", newCtrl)
	}
	return &NonNamespacedController[T, TList]{newCtrl}, nil
}

// Cache calls ControllerInterface.Cache(...) and wraps the result in a new NonNamespacedCache.
func (c *NonNamespacedController[T, TList]) Cache() NonNamespacedCacheInterface[T] {
	return &NonNamespacedCache[T]{
		CacheInterface: c.Controller.Cache(),
	}
}

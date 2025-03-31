package ext

import (
	"context"
	"errors"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

var (
	errMissingUserInfo = errors.New("missing user info")
)

// delegate is the bridge between k8s.io/apiserver's [rest.Storage] interface and
// our own Store interface we want developers to use
//
// It currently supports non-namespaced stores only because Store[T, TList] doesn't
// expose namespaces anywhere. When needed we'll add support to namespaced resources.
type delegate[T runtime.Object, TList runtime.Object] struct {
	scheme *runtime.Scheme
	// t is the resource of the delegate (eg: *Token) and must be non-nil.
	t T
	// tList is the resource list of the delegate (eg: *TokenList) and must be non-nil.
	tList        TList
	gvk          schema.GroupVersionKind
	gvr          schema.GroupVersionResource
	singularName string
	store        Store[T, TList]
	authorizer   authorizer.Authorizer
}

// New implements [rest.Storage]
//
// It uses generics to create the resource and set its GVK.
func (s *delegate[T, TList]) New() runtime.Object {
	t := s.t.DeepCopyObject()
	t.GetObjectKind().SetGroupVersionKind(s.gvk)
	return t
}

// Destroy cleans up its resources on shutdown.
// Destroy has to be implemented in thread-safe way and be prepared
// for being called more than once.
//
// It is NOT meant to delete resources from the backing storage. It is meant to
// stop clients, runners, etc that could be running for the store when the extension
// API server gracefully shutdowns/exits.
func (s *delegate[T, TList]) Destroy() {
}

// NewList implements [rest.Lister]
//
// It uses generics to create the resource and set its GVK.
func (s *delegate[T, TList]) NewList() runtime.Object {
	tList := s.tList.DeepCopyObject()
	tList.GetObjectKind().SetGroupVersionKind(s.gvk)
	return tList
}

// List implements [rest.Lister]
func (s *delegate[T, TList]) List(parentCtx context.Context, internaloptions *metainternalversion.ListOptions) (runtime.Object, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, err
	}

	options, err := s.convertListOptions(internaloptions)
	if err != nil {
		return nil, err
	}

	return s.store.List(ctx, options)
}

// ConvertToTable implements [rest.Lister]
//
// It converts an object or a list of objects to a table, which is used by kubectl
// (and Rancher UI) to display a table of the items.
//
// Currently, we use the default table convertor which will show two columns: Name and Created At.
func (s *delegate[T, TList]) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	defaultTableConverter := rest.NewDefaultTableConvertor(s.gvr.GroupResource())
	return defaultTableConverter.ConvertToTable(ctx, object, tableOptions)
}

// Get implements [rest.Getter]
func (s *delegate[T, TList]) Get(parentCtx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, err
	}

	return s.store.Get(ctx, name, options)
}

// Delete implements [rest.GracefulDeleter]
//
// deleteValidation is used to do some validation on the object before deleting
// it in the store. For example, running mutating/validating webhooks, though we're not using these yet.
func (s *delegate[T, TList]) Delete(parentCtx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, false, err
	}

	oldObj, err := s.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err = deleteValidation(ctx, oldObj); err != nil {
			return nil, false, err
		}
	}

	err = s.store.Delete(ctx, name, options)
	return oldObj, true, err
}

// Create implements [rest.Creater]
//
// createValidation is used to do some validation on the object before creating
// it in the store. For example, running mutating/validating webhooks, though we're not using these yet.
//
//nolint:misspell
func (s *delegate[T, TList]) Create(parentCtx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, err
	}

	if createValidation != nil {
		err := createValidation(ctx, obj)
		if err != nil {
			return obj, err
		}
	}

	tObj, ok := obj.(T)
	if !ok {
		return nil, fmt.Errorf("object was of type %T, not of expected type %T", obj, s.t)
	}

	return s.store.Create(ctx, tObj, options)
}

// Update implements [rest.Updater]
//
// createValidation is used to do some validation on the object before creating
// it in the store. For example, it will do an authorization check for "create"
// verb if the object needs to be created.
// See here for details: https://github.com/kubernetes/apiserver/blob/70ed6fdbea9eb37bd1d7558e90c20cfe888955e8/pkg/endpoints/handlers/update.go#L190-L201
// Another example is running mutating/validating webhooks, though we're not using these yet.
//
// updateValidation is used to do some validation on the object before updating it in the store.
// One example is running mutating/validating webhooks, though we're not using these yet.
func (s *delegate[T, TList]) Update(
	parentCtx context.Context,
	name string,
	objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc,
	updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool,
	options *metav1.UpdateOptions,
) (runtime.Object, bool, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, false, err
	}

	oldObj, err := s.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		obj, err := objInfo.UpdatedObject(ctx, nil)
		if err != nil {
			return nil, false, err
		}

		if err = createValidation(ctx, obj); err != nil {
			return nil, false, err
		}

		tObj, ok := obj.(T)
		if !ok {
			return nil, false, fmt.Errorf("object was of type %T, not of expected type %T", obj, s.t)
		}

		newObj, err := s.store.Create(ctx, tObj, &metav1.CreateOptions{})
		if err != nil {
			return nil, false, err
		}
		return newObj, true, err
	}

	newObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}

	newT, ok := newObj.(T)
	if !ok {
		return nil, false, fmt.Errorf("object was of type %T, not of expected type %T", newObj, s.t)
	}

	if updateValidation != nil {
		err = updateValidation(ctx, newT, oldObj)
		if err != nil {
			return nil, false, err
		}
	}

	newT, err = s.store.Update(ctx, newT, options)
	if err != nil {
		return nil, false, err
	}

	return newT, false, nil
}

type watcher struct {
	closedLock sync.RWMutex
	closed     bool
	ch         chan watch.Event
}

func (w *watcher) Stop() {
	w.closedLock.Lock()
	defer w.closedLock.Unlock()
	if !w.closed {
		close(w.ch)
		w.closed = true
	}
}

func (w *watcher) addEvent(event watch.Event) bool {
	w.closedLock.RLock()
	defer w.closedLock.RUnlock()
	if w.closed {
		return false
	}

	w.ch <- event
	return true
}

func (w *watcher) ResultChan() <-chan watch.Event {
	return w.ch
}

func (s *delegate[T, TList]) Watch(parentCtx context.Context, internaloptions *metainternalversion.ListOptions) (watch.Interface, error) {
	ctx, err := s.makeContext(parentCtx)
	if err != nil {
		return nil, err
	}

	options, err := s.convertListOptions(internaloptions)
	if err != nil {
		return nil, err
	}

	w := &watcher{
		ch: make(chan watch.Event),
	}
	go func() {
		// Not much point continuing the watch if the store stopped its watch.
		// Double stopping here is fine.
		defer w.Stop()

		// Closing eventCh is the responsibility of the store.Watch method
		// to avoid the store panicking while trying to send to a close channel
		eventCh, err := s.store.Watch(ctx, options)
		if err != nil {
			return
		}

		for event := range eventCh {
			added := w.addEvent(watch.Event{
				Type:   event.Event,
				Object: event.Object,
			})
			if !added {
				break
			}
		}
	}()

	return w, nil
}

// GroupVersionKind implements rest.GroupVersionKind
//
// This is used to generate the data for the Discovery API
func (s *delegate[T, TList]) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	return s.gvk
}

// NamespaceScoped implements rest.Scoper
//
// The delegate is used for non-namespaced resources so it always returns false
func (s *delegate[T, TList]) NamespaceScoped() bool {
	return false
}

// Kind implements rest.KindProvider
//
// XXX: Example where / how this is used
func (s *delegate[T, TList]) Kind() string {
	return s.gvk.Kind
}

// GetSingularName implements rest.SingularNameProvider
//
// This is used by a variety of things such as kubectl to map singular name to
// resource name. (eg: token => tokens)
func (s *delegate[T, TList]) GetSingularName() string {
	return s.singularName
}

func (s *delegate[T, TList]) makeContext(parentCtx context.Context) (Context, error) {
	userInfo, ok := request.UserFrom(parentCtx)
	if !ok {
		return Context{}, errMissingUserInfo
	}

	ctx := Context{
		Context:              parentCtx,
		User:                 userInfo,
		Authorizer:           s.authorizer,
		GroupVersionResource: s.gvr,
	}
	return ctx, nil
}

func (s *delegate[T, TList]) convertListOptions(options *metainternalversion.ListOptions) (*metav1.ListOptions, error) {
	var out metav1.ListOptions
	err := s.scheme.Convert(options, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("convert list options: %w", err)
	}

	return &out, nil
}

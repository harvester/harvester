package ext

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"
)

// delegateError wraps an inner delegate and converts unknown errors.
type delegateError[T runtime.Object, TList runtime.Object] struct {
	inner *delegate[T, TList]
}

func (d *delegateError[T, TList]) New() runtime.Object {
	return d.inner.New()
}

func (d *delegateError[T, TList]) Destroy() {
	d.inner.Destroy()
}

func (d *delegateError[T, TList]) NewList() runtime.Object {
	return d.inner.NewList()
}

func (d *delegateError[T, TList]) List(parentCtx context.Context, internaloptions *metainternalversion.ListOptions) (runtime.Object, error) {
	result, err := d.inner.List(parentCtx, internaloptions)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

func (d *delegateError[T, TList]) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	result, err := d.inner.ConvertToTable(ctx, object, tableOptions)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

func (d *delegateError[T, TList]) Get(parentCtx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	result, err := d.inner.Get(parentCtx, name, options)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

func (d *delegateError[T, TList]) Delete(parentCtx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	result, completed, err := d.inner.Delete(parentCtx, name, deleteValidation, options)
	if err != nil {
		return nil, false, convertError(err)
	}
	return result, completed, nil
}

func (d *delegateError[T, TList]) Create(parentCtx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	result, err := d.inner.Create(parentCtx, obj, createValidation, options)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

func (d *delegateError[T, TList]) Update(parentCtx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	result, created, err := d.inner.Update(parentCtx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
	if err != nil {
		return nil, false, convertError(err)
	}
	return result, created, nil
}

func (d *delegateError[T, TList]) Watch(parentCtx context.Context, internaloptions *metainternalversion.ListOptions) (watch.Interface, error) {
	result, err := d.inner.Watch(parentCtx, internaloptions)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

func (d *delegateError[T, TList]) GroupVersionKind(groupVersion schema.GroupVersion) schema.GroupVersionKind {
	return d.inner.GroupVersionKind(groupVersion)
}

func (d *delegateError[T, TList]) NamespaceScoped() bool {
	return d.inner.NamespaceScoped()
}

func (d *delegateError[T, TList]) Kind() string {
	return d.inner.Kind()
}

func (d *delegateError[T, TList]) GetSingularName() string {
	return d.inner.GetSingularName()
}

func convertError(err error) error {
	if _, ok := err.(errors.APIStatus); ok {
		return err
	}

	return errors.NewInternalError(err)
}

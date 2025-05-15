package ext

import (
	"context"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
)

// ConvertFunc will convert an object to a list of cell in a metav1.Table (think kubectl get table output)
type ConvertFunc[T runtime.Object] func(obj T) []string

// ConvertToTable helps implement [rest.Lister] and [rest.TableConvertor].
//
// It converts an object or a list of objects to a Table, which is used by kubectl
// (and Rancher UI) to display a table of the items.
func ConvertToTable[T runtime.Object](ctx context.Context, object runtime.Object, tableOptions runtime.Object, groupResource schema.GroupResource, columnDefs []metav1.TableColumnDefinition, convertFn ConvertFunc[T]) (*metav1.Table, error) {
	result, err := convertToTable(ctx, object, tableOptions, groupResource, columnDefs, convertFn)
	if err != nil {
		return nil, convertError(err)
	}
	return result, nil
}

// ConvertToTableDefault helps implement [rest.Lister] and [rest.TableConvertor].
//
// This uses the default table conversion that displays the following two
// columns: Name and Created At.
func ConvertToTableDefault[T runtime.Object](ctx context.Context, object runtime.Object, tableOptions runtime.Object, groupResource schema.GroupResource) (*metav1.Table, error) {
	return ConvertToTable[T](ctx, object, tableOptions, groupResource, nil, nil)
}

func convertToTable[T runtime.Object](ctx context.Context, object runtime.Object, tableOptions runtime.Object, groupResource schema.GroupResource, columnDefs []metav1.TableColumnDefinition, convertFn ConvertFunc[T]) (*metav1.Table, error) {
	defaultTableConverter := rest.NewDefaultTableConvertor(groupResource)
	table, err := defaultTableConverter.ConvertToTable(ctx, object, tableOptions)
	if err != nil {
		return nil, err
	}

	if columnDefs == nil {
		return table, nil
	}

	// Override only if there were definitions before (to respect the NoHeader option)
	if len(table.ColumnDefinitions) > 0 {
		table.ColumnDefinitions = columnDefs
	}
	table.Rows = []metav1.TableRow{}
	fn := func(obj runtime.Object) error {
		objT, ok := obj.(T)
		if !ok {
			var zeroT T
			return fmt.Errorf("expected %T but got %T", zeroT, obj)
		}
		cells := convertFn(objT)
		if len(cells) != len(columnDefs) {
			return fmt.Errorf("defined %d columns but got %d cells", len(columnDefs), len(cells))
		}

		table.Rows = append(table.Rows, metav1.TableRow{
			Cells:  cellStringToCellAny(cells),
			Object: runtime.RawExtension{Object: obj},
		})
		return nil
	}
	switch {
	case meta.IsListType(object):
		if err := meta.EachListItem(object, fn); err != nil {
			return nil, err
		}
	default:
		if err := fn(object); err != nil {
			return nil, err
		}
	}

	return table, nil
}

func cellStringToCellAny(cells []string) []any {
	var res []any
	for _, cell := range cells {
		res = append(res, cell)
	}
	return res
}

// CreateOrUpdate helps implement [rest.Updater] by handling most of the logic.
//
// It will call getFn to find the object. If not found, then createFn will
// be called, which should create the object. Otherwise, the updateFn will be called,
// which should update the object.
//
// createValidation is called before createFn. It will do validation such as:
//   - verifying that the user is allowed to by checking for the "create" verb.
//     See here for details: https://github.com/kubernetes/apiserver/blob/70ed6fdbea9eb37bd1d7558e90c20cfe888955e8/pkg/endpoints/handlers/update.go#L190-L201
//   - running mutating/validating webhooks (though we're not using them yet)
//
// updateValidation is called before updateFn. It will do validation such as:
// - running mutating/validating webhooks (though we're not using them yet)
func CreateOrUpdate[T runtime.Object](
	ctx context.Context,
	name string,
	objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc,
	updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool,
	options *metav1.UpdateOptions,
	getFn func(ctx context.Context, name string, opts *metav1.GetOptions) (T, error),
	createFn func(ctx context.Context, obj T, opts *metav1.CreateOptions) (T, error),
	updateFn func(ctx context.Context, obj T, opts *metav1.UpdateOptions) (T, error),
) (runtime.Object, bool, error) {
	oldObj, err := getFn(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, false, err
		}

		obj, err := objInfo.UpdatedObject(ctx, nil)
		if err != nil {
			return nil, false, convertError(err)
		}

		if err = createValidation(ctx, obj); err != nil {
			return nil, false, convertError(err)
		}

		tObj, ok := obj.(T)
		if !ok {
			var zeroT T
			return nil, false, convertError(fmt.Errorf("object was of type %T, not of expected type %T", obj, zeroT))
		}

		newObj, err := createFn(ctx, tObj, &metav1.CreateOptions{})
		if err != nil {
			return nil, false, convertError(err)
		}
		return newObj, true, nil
	}

	newObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, convertError(err)
	}

	newT, ok := newObj.(T)
	if !ok {
		var zeroT T
		return nil, false, convertError(fmt.Errorf("object was of type %T, not of expected type %T", newObj, zeroT))
	}

	if updateValidation != nil {
		err = updateValidation(ctx, newT, oldObj)
		if err != nil {
			return nil, false, convertError(err)
		}
	}

	newT, err = updateFn(ctx, newT, options)
	if err != nil {
		return nil, false, err
	}

	return newT, false, nil
}

// ConvertListOptions converts an internal ListOptions to one used by client-go.
//
// This can be useful if wrapping Watch or List methods to client-go's equivalent.
func ConvertListOptions(options *metainternalversion.ListOptions) (*metav1.ListOptions, error) {
	scheme := sync.OnceValue(func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		metainternalversion.AddToScheme(scheme)
		return scheme
	})()

	var out metav1.ListOptions
	err := scheme.Convert(options, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("converting list options: %w", err)
	}

	return &out, nil
}

func convertError(err error) error {
	if _, ok := err.(apierrors.APIStatus); ok {
		return err
	}

	return apierrors.NewInternalError(err)
}

package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rancher/lasso/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	// recentDeletionCacheExpiration configures the duration for keeping a history of recently deleted object UIDs
	recentDeletionCacheExpiration = 1 * time.Minute
	// retryPeriodForRecentlyDeletedObject is the time to wait before retrying enqueuing a key that was just deleted but still present in the informer store
	retryPeriodForRecentlyDeletedObject = 10 * time.Second
)

var (
	ErrIgnore = errors.New("ignore handler error")
)

type handlerEntry struct {
	id      int64
	name    string
	handler SharedControllerHandler
}

type SharedHandler struct {
	// Used for metrics recording
	// They are exported because this SharedHandler is sometimes embedded used as a field in other packages, like dynamic
	ControllerName string
	CtxID          string

	// keep first because arm32 needs atomic.AddInt64 target to be mem aligned
	idCounter int64

	lock            sync.RWMutex
	handlers        []handlerEntry
	recentDeletions *cache.Expiring
}

func (h *SharedHandler) Register(ctx context.Context, name string, handler SharedControllerHandler) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.recentDeletions == nil {
		h.recentDeletions = cache.NewExpiring()
	}

	id := atomic.AddInt64(&h.idCounter, 1)
	h.handlers = append(h.handlers, handlerEntry{
		id:      id,
		name:    name,
		handler: handler,
	})

	go func() {
		<-ctx.Done()

		h.lock.Lock()
		defer h.lock.Unlock()

		for i := range h.handlers {
			if h.handlers[i].id == id {
				h.handlers = append(h.handlers[:i], h.handlers[i+1:]...)
				break
			}
		}
	}()
}

func (h *SharedHandler) OnChange(key string, obj runtime.Object) error {
	// early skip for a special case: objects that were just deleted but still not updated in the informer cache.
	// modifications performed by early chained handlers also cause a new enqueue of the processed key, while later late handlers modifications
	// could cause the definitive deletion of the object (by removing a finalizer). If this happens fast enough, it creates a race condition where handlers receive an out-of-date version of the object.
	// See https://github.com/rancher/rancher/issues/49328 for more details.
	if obj != nil && h.deletedInPreviousExecution(obj) {
		return &retryAfterError{duration: retryPeriodForRecentlyDeletedObject}
	}

	h.lock.RLock()
	handlers := h.handlers
	h.lock.RUnlock()

	var errs errorList
	for _, handler := range handlers {
		var hasError bool
		reconcileStartTS := time.Now()

		newObj, err := handler.handler.OnChange(key, obj)
		if err != nil && !errors.Is(err, ErrIgnore) {
			errs = append(errs, &handlerError{
				HandlerName: handler.name,
				Err:         err,
			})
			hasError = true
		}
		metrics.IncTotalHandlerExecutions(h.CtxID, h.ControllerName, handler.name, hasError)
		reconcileTime := time.Since(reconcileStartTS)
		metrics.ReportReconcileTime(h.CtxID, h.ControllerName, handler.name, hasError, reconcileTime.Seconds())

		if newObj != nil && !reflect.ValueOf(newObj).IsNil() {
			meta, err := meta.Accessor(newObj)
			if err == nil && meta.GetUID() != "" {
				// avoid using an empty object
				obj = newObj
			} else if err != nil {
				// assign if we can't determine metadata
				obj = newObj
			}
		}
	}

	if obj != nil && wasFinalized(obj) {
		h.observeDeletedObjectAfterFinalize(obj)
	}

	return errs.ToErr()
}

// wasFinalized determines if an object which initially had finalizers got them removed, hence unblocking its erasure by Kubernetes
// Caveats: deletionTimestamp is never set for objects without finalizers, as Kubernetes will directly delete the object instead
func wasFinalized(obj runtime.Object) bool {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return false
	}
	return meta.GetDeletionTimestamp() != nil && len(meta.GetFinalizers()) == 0
}

// observeDeletedObjectAfterFinalize will temporarily store the UID of an object, so successive executions of the handlers have "memory" of this event
func (h *SharedHandler) observeDeletedObjectAfterFinalize(obj runtime.Object) {
	// Corner-case: Register was never called, so recentDeletions was not initialized
	// Currently, since there is no constructor for SharedHandler, the only place where we can initialize this cache is the Register hook
	if h.recentDeletions == nil {
		return
	}

	meta, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	h.recentDeletions.Set(meta.GetUID(), struct{}{}, recentDeletionCacheExpiration)
}

// deletedInPreviousExecution returns whether an object has been deleted in earlier executions of the controller.
func (h *SharedHandler) deletedInPreviousExecution(obj runtime.Object) bool {
	// Corner-case: Register was never called, so recentDeletions was not initialized
	// Currently, since there is no constructor for SharedHandler, the only place where we can initialize this cache is the Register hook
	if h.recentDeletions == nil {
		return false
	}

	meta, err := meta.Accessor(obj)
	if err != nil {
		return false
	}

	// avoid checking the cache for objects not marked for deletion
	if meta.GetDeletionTimestamp() == nil {
		return false
	}

	_, ok := h.recentDeletions.Get(meta.GetUID())
	return ok
}

type errorList []error

func (e errorList) Error() string {
	buf := strings.Builder{}
	for _, err := range e {
		if buf.Len() > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(err.Error())
	}
	return buf.String()
}

func (e errorList) ToErr() error {
	switch len(e) {
	case 0:
		return nil
	case 1:
		return e[0]
	default:
		return e
	}
}

func (e errorList) Cause() error {
	if len(e) > 0 {
		return e[0]
	}
	return nil
}

type handlerError struct {
	HandlerName string
	Err         error
}

func (h handlerError) Error() string {
	return fmt.Sprintf("handler %s: %v", h.HandlerName, h.Err)
}

func (h handlerError) Cause() error {
	return h.Err
}

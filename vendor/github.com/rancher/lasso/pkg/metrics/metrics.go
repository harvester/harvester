package metrics

import (
	"context"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var prometheusMetrics = false

const (
	lassoSubsystem      = "lasso_controller"
	controllerNameLabel = "controller_name"
	handlerNameLabel    = "handler_name"
	hasErrorLabel       = "has_error"

	contextLabel = "ctx"
	groupLabel   = "group"
	versionLabel = "version"
	kindLabel    = "kind"
)

type contextIDKey struct{}

// WithContextID stores an identifier within the Context for later use when collecting metrics
func WithContextID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextIDKey{}, id)
}

// ContextID extracts the identifier previously set by WithContextID, returning an empty string otherwise
func ContextID(ctx context.Context) string {
	id, _ := ctx.Value(contextIDKey{}).(string)
	return id
}

var (
	// https://prometheus.io/docs/practices/instrumentation/#use-labels explains logic of having 1 total_requests
	// counter with code label vs a counter for each code

	TotalControllerExecutions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: lassoSubsystem,
			Name:      "total_handler_execution",
			Help:      "Total count of handler executions",
		},
		[]string{controllerNameLabel, handlerNameLabel, hasErrorLabel},
	)
	TotalCachedObjects = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: lassoSubsystem,
			Name:      "total_cached_object",
			Help:      "Total count of cached objects",
		},
		[]string{contextLabel, groupLabel, versionLabel, kindLabel},
	)

	// reconcileTime is a prometheus histogram metric exposes the duration of reconciliations per controller.
	// controller label refers to the controller name
	reconcileTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: lassoSubsystem,
		Name:      "reconcile_time_seconds",
		Help:      "Histogram of the durations per reconciliation per controller",
	}, []string{controllerNameLabel, handlerNameLabel, hasErrorLabel})
)

func IncTotalHandlerExecutions(controllerName, handlerName string, hasError bool) {
	if prometheusMetrics {
		TotalControllerExecutions.With(
			prometheus.Labels{
				controllerNameLabel: controllerName,
				handlerNameLabel:    handlerName,
				hasErrorLabel:       strconv.FormatBool(hasError),
			},
		).Inc()
	}
}

// IncTotalCachedObjects sets the cached items count for the specified context and GroupVersionKind
func IncTotalCachedObjects(ctxID string, gvk schema.GroupVersionKind, count int) {
	if prometheusMetrics {
		TotalCachedObjects.With(
			prometheus.Labels{
				contextLabel: ctxID,
				groupLabel:   gvk.Group,
				versionLabel: gvk.Version,
				kindLabel:    gvk.Kind,
			},
		).Set(float64(count))
	}
}

// DelTotalCachedObjects deletes the cached items count metric matching the provided content and GroupVersionKind
func DelTotalCachedObjects(ctxID string, gvk schema.GroupVersionKind) {
	if prometheusMetrics {
		TotalCachedObjects.Delete(
			prometheus.Labels{
				contextLabel: ctxID,
				groupLabel:   gvk.Group,
				versionLabel: gvk.Version,
				kindLabel:    gvk.Kind,
			},
		)
	}
}

func ReportReconcileTime(controllerName, handlerName string, hasError bool, observeTime float64) {
	if prometheusMetrics {
		reconcileTime.With(
			prometheus.Labels{
				controllerNameLabel: controllerName,
				handlerNameLabel:    handlerName,
				hasErrorLabel:       strconv.FormatBool(hasError),
			},
		).Observe(observeTime)
	}
}

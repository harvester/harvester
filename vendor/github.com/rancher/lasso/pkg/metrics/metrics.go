package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var prometheusMetrics = false

const (
	lassoSubsystem      = "lasso_controller"
	controllerNameLabel = "controller_name"
	handlerNameLabel    = "handler_name"
	hasErrorLabel       = "has_error"

	groupLabel   = "group"
	versionLabel = "version"
	kindLabel    = "kind"
)

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
		[]string{groupLabel, versionLabel, kindLabel},
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

func IncTotalCachedObjects(group, version, kind string, val float64) {
	if prometheusMetrics {
		TotalCachedObjects.With(
			prometheus.Labels{
				groupLabel:   group,
				versionLabel: version,
				kindLabel:    kind,
			},
		).Set(val)
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

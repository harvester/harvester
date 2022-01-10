package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// reconcile result
	ReconcileResultSuccess     = "success"
	ReconcileResultError       = "error"
	ReconcileResultErrorIgnore = "error_ignore"
	// subsystem name
	controllerSubsystem = "controller"
	// metrics name
	reconcileTotalKey       = "reconcile_total"
	reconcileTimeSecondsKey = "reconcile_time_seconds"
)

var (
	// reconcileTotal is a prometheus counter metrics exposes the total number of reconciliations per controller.
	// controller label refers to the controller name and result label refers to the reconcile result.
	reconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: controllerSubsystem,
		Name:      reconcileTotalKey,
		Help:      "Total number of reconciliations per controller",
	}, []string{"controller", "result"})

	// reconcileTime is a prometheus histogram metric exposes the duration of reconciliations per controller.
	// controller label refers to the controller name
	reconcileTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: controllerSubsystem,
		Name:      reconcileTimeSecondsKey,
		Help:      "Histogram of the durations per reconciliation per controller",
	}, []string{"controller"})
)

func ReportReconcileTotal(labels ...string) {
	if prometheusMetrics {
		reconcileTotal.WithLabelValues(labels...).Inc()
	}
}

func ReportReconcileTime(observeTime float64, labels ...string) {
	if prometheusMetrics {
		reconcileTime.WithLabelValues(labels...).Observe(observeTime)
	}
}

package metrics

import (
	"os"

	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsEnv = "CATTLE_PROMETHEUS_METRICS"

func init() {
	if os.Getenv(metricsEnv) == "true" {
		MustRegisterWithWorkqueue(prometheus.DefaultRegisterer)
	}
}

func Enabled() bool {
	return prometheusMetrics
}

// MustRegisterWithWorkqueue registers all metrics, including Kubernetes internal
// workqueue metrics, with the provided registerer
func MustRegisterWithWorkqueue(registerer prometheus.Registerer) {
	prometheusMetrics = true
	registerer.MustRegister(
		TotalControllerExecutions,
		TotalCachedObjects,
		reconcileTime,
		// expose workqueue metrics
		depth,
		adds,
		latency,
		workDuration,
		unfinished,
		longestRunningProcessor,
		retries,
	)
	workqueue.SetProvider(workqueueMetricsProvider{})
}

// MustRegister registers only lasso-specific metrics.
// This must be used if attempting to register Lasso metrics to the same
// registerer as used by Kubernetes itself, otherwise prometheus will panic
// when both packages register metrics with the same name.
func MustRegister(registerer prometheus.Registerer) {
	prometheusMetrics = true
	registerer.MustRegister(
		TotalControllerExecutions,
		TotalCachedObjects,
		reconcileTime,
	)
}

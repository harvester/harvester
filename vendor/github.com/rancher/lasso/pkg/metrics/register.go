package metrics

import (
	"os"

	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsEnv = "CATTLE_PROMETHEUS_METRICS"

func init() {
	if os.Getenv(metricsEnv) == "true" {
		prometheusMetrics = true
		prometheus.MustRegister(
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
}

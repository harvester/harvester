package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"k8s.io/client-go/util/workqueue"
)

const metricsEnv = "EXPOSE_PROMETHEUS_METRICS"

var (
	// Registry stored metrics using lasso controller framework
	Registry          = prometheus.NewRegistry()
	prometheusMetrics = false
)

func init() {
	if os.Getenv(metricsEnv) != "true" {
		return
	}

	prometheusMetrics = true
	// register metrics
	Registry.MustRegister(
		// expose controller reconciliation metrics
		reconcileTotal,
		reconcileTime,
		// expose workqueue metrics
		depth,
		adds,
		latency,
		workDuration,
		unfinished,
		longestRunningProcessor,
		retries,
		// expose process metrics.
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		// expose Go runtime metrics.
		collectors.NewGoCollector(),
	)

	workqueue.SetProvider(workqueueMetricsProvider{})
}

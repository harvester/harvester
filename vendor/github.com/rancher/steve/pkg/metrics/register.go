package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsEnv = "CATTLE_PROMETHEUS_METRICS"

func init() {
	if os.Getenv(metricsEnv) == "true" {
		prometheusMetrics = true
		prometheus.MustRegister(ProxyTotalResponses)
		prometheus.MustRegister(K8sClientResponseTime)
		prometheus.MustRegister(ProxyStoreResponseTime)
		prometheus.MustRegister(SqliteDBSizeMetrics)
		prometheus.MustRegister(SqliteDBWalSizeMetrics)
		prometheus.MustRegister(SqliteDBShmSizeMetrics)
	}
}

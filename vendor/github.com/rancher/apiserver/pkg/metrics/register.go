package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsEnv = "CATTLE_PROMETHEUS_METRICS"

func init() {
	if os.Getenv(metricsEnv) == "true" {
		prometheusMetrics = true
		prometheus.MustRegister(TotalResponses)
		prometheus.MustRegister(ResponseTime)
	}
}

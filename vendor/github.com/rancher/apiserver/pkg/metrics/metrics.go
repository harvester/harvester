package metrics

import "github.com/prometheus/client_golang/prometheus"

var prometheusMetrics = false

const (
	resourceLabel = "resource"
	methodLabel   = "method"
	codeLabel     = "code"
)
var (
	// https://prometheus.io/docs/practices/instrumentation/#use-labels explains logic of having 1 total_requests
	// counter with code label vs a counter for each code

	TotalResponses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "steve_api",
			Name:      "total_requests",
			Help:      "Total count API requests",
		},
		[]string{resourceLabel, methodLabel, codeLabel},
	)

	ResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "steve_api",
			Name:      "request_time",
			Help:      "Request times in ms",
		},
		[]string{resourceLabel, methodLabel, codeLabel})
)

func IncTotalResponses(resource, method, code string) {
	if prometheusMetrics {
		TotalResponses.With(
			prometheus.Labels{
				resourceLabel: resource,
				methodLabel:   method,
				codeLabel:     code,
			},
		).Inc()
	}
}

func RecordResponseTime(resource, method, code string, val float64) {
	if prometheusMetrics {
		ResponseTime.With(
			prometheus.Labels{
				resourceLabel: resource,
				methodLabel:   method,
				codeLabel:     code,
			},
		).Observe(val)
	}
}

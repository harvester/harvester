package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/apiserver/pkg/apierror"
)

type MetricLogger struct {
	Resource string
	Method   string
}

var prometheusMetrics = false

const (
	resourceLabel = "resource"
	methodLabel   = "method"
	codeLabel     = "code"
)

var (
	// https://prometheus.io/docs/practices/instrumentation/#use-labels explains logic of having 1 total_requests
	// counter with code label vs a counter for each code

	ProxyTotalResponses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "k8s_proxy",
			Name:      "total_requests",
			Help:      "Total count API requests",
		},
		[]string{resourceLabel, methodLabel, codeLabel},
	)
	K8sClientResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "k8s_proxy",
			Name:      "client_request_time",
			Help:      "Request times in ms for k8s client from proxy store",
		},
		[]string{resourceLabel, methodLabel, codeLabel})
	ProxyStoreResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "k8s_proxy",
			Name:      "store_request_time",
			Help:      "Request times in ms for k8s proxy store",
		},
		[]string{resourceLabel, methodLabel, codeLabel})
)

func (m MetricLogger) IncTotalResponses(err error) {
	if prometheusMetrics {
		ProxyTotalResponses.With(
			prometheus.Labels{
				resourceLabel: m.Resource,
				methodLabel:   m.Method,
				codeLabel:     m.getAPIErrorCode(err),
			},
		).Inc()
	}
}

func (m MetricLogger) RecordK8sClientResponseTime(err error, val float64) {
	if prometheusMetrics {
		K8sClientResponseTime.With(
			prometheus.Labels{
				resourceLabel: m.Resource,
				methodLabel:   m.Method,
				codeLabel:     m.getAPIErrorCode(err),
			},
		).Observe(val)
	}
}

func (m MetricLogger) RecordProxyStoreResponseTime(err error, val float64) {
	if prometheusMetrics {
		ProxyStoreResponseTime.With(
			prometheus.Labels{
				resourceLabel: m.Resource,
				methodLabel:   m.Method,
				codeLabel:     m.getAPIErrorCode(err),
			},
		).Observe(val)
	}
}

func (m MetricLogger) getAPIErrorCode(err error) string {
	successCode := "200"
	if m.Method == http.MethodPost {
		successCode = "201"
	}
	if err == nil {
		return successCode
	}
	if apiError, ok := err.(*apierror.APIError); ok {
		return strconv.Itoa(apiError.Code.Status)
	}
	return "500"
}

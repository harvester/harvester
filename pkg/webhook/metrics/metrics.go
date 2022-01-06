package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/lasso/pkg/metrics"
)

const (
	// subsystem name
	HarvesterSubsystem = "harvester"
	// metrics name
	WebhookLatencySecondsKey   = "webhook_latency_seconds"
	WebhookRequestsTotalKey    = "webhook_requests_total"
	WebhookRequestsInFlightKey = "webhook_requests_in_flight"
)

var (
	// ReconcileTime is a prometheus histogram metric exposes the duration of processing admission requests.
	RequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: HarvesterSubsystem,
			Name:      WebhookLatencySecondsKey,
			Help:      "Histogram of the latency of processing admission requests",
		},
		[]string{"webhook"},
	)

	// RequestTotal is a prometheus counter metric expose the total processed admission requests.
	RequestTotal = func() *prometheus.CounterVec {
		return prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: HarvesterSubsystem,
				Name:      WebhookRequestsTotalKey,
				Help:      "Total number of admission requests by HTTP status code.",
			},
			[]string{"webhook", "code"},
		)
	}()

	// RequestInFlight is a prometheus gauge metric expose the in-flight admission requests.
	RequestInFlight = func() *prometheus.GaugeVec {
		return prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: HarvesterSubsystem,
				Name:      WebhookRequestsInFlightKey,
				Help:      "Current number of admission requests being served.",
			},
			[]string{"webhook"},
		)
	}()
)

func init() {
	metrics.Registry.MustRegister(RequestLatency, RequestTotal, RequestInFlight)
}

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Middleware is a middleware that wraps the provided http.Handler to observe the request
type Middleware struct {
	enabled bool
}

func NewMiddleware(enabled bool) *Middleware {
	return &Middleware{enabled: enabled}
}

func (m *Middleware) MetricsInstrument(path string, next http.Handler) http.Handler {
	if !m.enabled {
		return next
	}

	lbl := prometheus.Labels{"webhook": path}

	rnfHandler := promhttp.InstrumentHandlerInFlight(RequestInFlight.With(lbl), next)
	rtHandler := promhttp.InstrumentHandlerCounter(RequestTotal.MustCurryWith(lbl), rnfHandler)
	return promhttp.InstrumentHandlerDuration(RequestLatency.MustCurryWith(lbl), rtHandler)
}

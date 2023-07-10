package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	promNamespace = "goresilience"

	promCommandSubsystem          = "command"
	promRetrySubsystem            = "retry"
	promTimeoutSubsystem          = "timeout"
	promBulkheadSubsystem         = "bulkhead"
	promCBSubsystem               = "circuitbreaker"
	promChaosSubsystem            = "chaos"
	promConcurrencyLimitSubsystem = "concurrencylimit"
)

type prometheusRec struct {
	// Metrics.
	cmdExecutionDuration      *prometheus.HistogramVec
	retryRetries              *prometheus.CounterVec
	timeoutTimeouts           *prometheus.CounterVec
	bulkQueued                *prometheus.CounterVec
	bulkProcessed             *prometheus.CounterVec
	bulkTimeouts              *prometheus.CounterVec
	cbStateChanges            *prometheus.CounterVec
	chaosFailureInjections    *prometheus.CounterVec
	concurrencyLimitInflights *prometheus.GaugeVec
	concurrencyLimitResult    *prometheus.CounterVec
	concurrencyLimitLimit     *prometheus.GaugeVec

	id  string
	reg prometheus.Registerer
}

// NewPrometheusRecorder returns a new Recorder that knows how to measure
// using Prometheus kind metrics.
func NewPrometheusRecorder(reg prometheus.Registerer) Recorder {
	p := &prometheusRec{
		reg: reg,
	}

	p.registerMetrics()
	return p
}

func (p prometheusRec) WithID(id string) Recorder {
	return &prometheusRec{
		cmdExecutionDuration:      p.cmdExecutionDuration,
		retryRetries:              p.retryRetries,
		timeoutTimeouts:           p.timeoutTimeouts,
		bulkQueued:                p.bulkQueued,
		bulkProcessed:             p.bulkProcessed,
		bulkTimeouts:              p.bulkTimeouts,
		cbStateChanges:            p.cbStateChanges,
		chaosFailureInjections:    p.chaosFailureInjections,
		concurrencyLimitInflights: p.concurrencyLimitInflights,
		concurrencyLimitResult:    p.concurrencyLimitResult,
		concurrencyLimitLimit:     p.concurrencyLimitLimit,

		id:  id,
		reg: p.reg,
	}
}

func (p *prometheusRec) registerMetrics() {
	p.cmdExecutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: promNamespace,
		Subsystem: promCommandSubsystem,
		Name:      "execution_duration_seconds",
		Help:      "The duration of the command execution in seconds.",
	}, []string{"id", "success"})

	p.retryRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promRetrySubsystem,
		Name:      "retries_total",
		Help:      "Total number of retries made by the retry runner.",
	}, []string{"id"})

	p.timeoutTimeouts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promTimeoutSubsystem,
		Name:      "timeouts_total",
		Help:      "Total number of timeouts made by the timeout runner.",
	}, []string{"id"})

	p.bulkQueued = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promBulkheadSubsystem,
		Name:      "queued_total",
		Help:      "Total number of queued funcs made by the bulkhead runner.",
	}, []string{"id"})

	p.bulkProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promBulkheadSubsystem,
		Name:      "processed_total",
		Help:      "Total number of processed funcs made by the bulkhead runner.",
	}, []string{"id"})

	p.bulkTimeouts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promBulkheadSubsystem,
		Name:      "timeouts_total",
		Help:      "Total number of timeouts funcs waiting for execution made by the bulkhead runner.",
	}, []string{"id"})

	p.cbStateChanges = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promCBSubsystem,
		Name:      "state_changes_total",
		Help:      "Total number of state changes made by the circuit breaker runner.",
	}, []string{"id", "state"})

	p.chaosFailureInjections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promChaosSubsystem,
		Name:      "failure_injections_total",
		Help:      "Total number of failure injectionsmade by the chaos runner.",
	}, []string{"id", "kind"})

	p.concurrencyLimitInflights = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: promConcurrencyLimitSubsystem,
		Name:      "inflight_executions",
		Help:      "The number of inflight executions, these are executing and queued.",
	}, []string{"id"})

	p.concurrencyLimitResult = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promConcurrencyLimitSubsystem,
		Name:      "result_total",
		Help:      "Total results of the executions measured by the limiter algorithm.",
	}, []string{"id", "result"})

	p.concurrencyLimitLimit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: promConcurrencyLimitSubsystem,
		Name:      "limiter_limit",
		Help:      "The concurrency limit measured and calculated by the limiter algorithm.",
	}, []string{"id"})

	p.reg.MustRegister(p.cmdExecutionDuration,
		p.retryRetries,
		p.timeoutTimeouts,
		p.bulkQueued,
		p.bulkProcessed,
		p.bulkTimeouts,
		p.cbStateChanges,
		p.chaosFailureInjections,
		p.concurrencyLimitInflights,
		p.concurrencyLimitResult,
		p.concurrencyLimitLimit,
	)
}

func (p prometheusRec) ObserveCommandExecution(start time.Time, success bool) {
	secs := time.Since(start).Seconds()
	p.cmdExecutionDuration.WithLabelValues(p.id, fmt.Sprintf("%t", success)).Observe(secs)
}

func (p prometheusRec) IncRetry() {
	p.retryRetries.WithLabelValues(p.id).Inc()
}

func (p prometheusRec) IncTimeout() {
	p.timeoutTimeouts.WithLabelValues(p.id).Inc()
}

func (p prometheusRec) IncBulkheadQueued() {
	p.bulkQueued.WithLabelValues(p.id).Inc()
}

func (p prometheusRec) IncBulkheadProcessed() {
	p.bulkProcessed.WithLabelValues(p.id).Inc()
}

func (p prometheusRec) IncBulkheadTimeout() {
	p.bulkTimeouts.WithLabelValues(p.id).Inc()
}

func (p prometheusRec) IncCircuitbreakerState(state string) {
	p.cbStateChanges.WithLabelValues(p.id, state).Inc()
}

func (p prometheusRec) IncChaosInjectedFailure(kind string) {
	p.chaosFailureInjections.WithLabelValues(p.id, kind).Inc()
}

func (p prometheusRec) SetConcurrencyLimitInflightExecutions(q int) {
	p.concurrencyLimitInflights.WithLabelValues(p.id).Set(float64(q))
}
func (p prometheusRec) IncConcurrencyLimitResult(result string) {
	p.concurrencyLimitResult.WithLabelValues(p.id, result).Inc()
}
func (p prometheusRec) SetConcurrencyLimitLimiterLimit(limit int) {
	p.concurrencyLimitLimit.WithLabelValues(p.id).Set(float64(limit))
}

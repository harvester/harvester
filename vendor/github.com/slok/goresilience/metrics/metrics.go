package metrics

import "time"

// Recorder knows how to measure different kind of metrics.
type Recorder interface {
	// WithID will set the ID name to the recorde and every metric
	// measured with the obtained recorder will be identified with
	// the name.
	WithID(id string) Recorder
	// ObserveCommandExecution will measure the exeuction of the runner chain.
	ObserveCommandExecution(start time.Time, success bool)
	// IncRetry will increment the number of retries.
	IncRetry()
	// IncTimeout will increment the number of timeouts.
	IncTimeout()
	// IncBulkheadQueued increments the number of queued Funcs to execute.
	IncBulkheadQueued()
	// IncBulkheadProcessed increments the number of processed Funcs to execute.
	IncBulkheadProcessed()
	// IncBulkheadProcessed increments the number of timeouts Funcs waiting  to execute.
	IncBulkheadTimeout()
	// IncCircuitbreakerState increments the number of state change.
	IncCircuitbreakerState(state string)
	// IncChaosInjectedFailure increments the number of times injected failure.
	IncChaosInjectedFailure(kind string)
	// SetConcurrencyLimitInflightExecutions sets the number of executions at a given moment
	// executing and queued for execution.
	SetConcurrencyLimitInflightExecutions(q int)
	// IncConcurrencyLimitResult increments the results obtained by the excutions after aplying the
	// limiter result policy.
	IncConcurrencyLimitResult(result string)
	// SetConcurrencyLimitLimiterLimit sets the current limit the limiter algorithm has calculated.
	SetConcurrencyLimitLimiterLimit(limit int)
}

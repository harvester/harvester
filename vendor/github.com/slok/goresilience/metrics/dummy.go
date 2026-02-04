package metrics

import "time"

// Dummy is a dummy recorder.
var Dummy Recorder = &dummy{}

type dummy struct{}

func (d *dummy) WithID(id string) Recorder                          { return d }
func (dummy) ObserveCommandExecution(start time.Time, success bool) {}
func (dummy) IncRetry()                                             {}
func (dummy) IncTimeout()                                           {}
func (dummy) IncBulkheadQueued()                                    {}
func (dummy) IncBulkheadProcessed()                                 {}
func (dummy) IncBulkheadTimeout()                                   {}
func (dummy) IncCircuitbreakerState(state string)                   {}
func (dummy) IncChaosInjectedFailure(kind string)                   {}
func (dummy) SetConcurrencyLimitInflightExecutions(q int)           {}
func (dummy) IncConcurrencyLimitResult(result string)               {}
func (dummy) SetConcurrencyLimitLimiterLimit(limit int)             {}

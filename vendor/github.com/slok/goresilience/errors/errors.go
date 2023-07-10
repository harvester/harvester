package errors

// Error is a goresilience error. Although satisfies Error interface from Golang
// this gives us the ability to check if the error was triggered by a Runner or not.
type Error string

func (e Error) Error() string {
	return string(e)
}

var (
	// ErrTimeout will be used when a execution timesout.
	ErrTimeout = Error("timeout while executing")
	// ErrContextCanceled will be used when the execution has not been executed due to the
	// context cancellation.
	ErrContextCanceled = Error("context canceled, logic not executed")
	// ErrTimeoutWaitingForExecution will be used when a execution block exceeded waiting
	// to be executed, for example if a worker pool has been busy and the execution object
	// has been waiting to much for being picked by a pool worker.
	ErrTimeoutWaitingForExecution = Error("timeout while waiting for execution")
	// ErrCircuitOpen will be used when a a circuit breaker is open.
	ErrCircuitOpen = Error("request rejected due to the circuit breaker being open")
	// ErrFailureInjected will be used when the chaos runner decides to inject error failure.
	ErrFailureInjected = Error("failure injected on purpose")
	// ErrRejectedExecution will be used by the executors when the execution of a func has been rejected
	// before being executed.
	ErrRejectedExecution = Error("execution has been rejected")
)

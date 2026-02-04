// Package goresilience is a framework/lirbary of utilities to improve the resilience of
// programs easily.
//
// The library is based on `goresilience.Runner` interface, this runners can be
// chained using the decorator pattern (like std library `http.Handler` interface).
// This makes the library being extensible, flexible and clean to use.
// The runners can be chained like if they were middlewares that could act on
// all the execution process of the `goresilience.Func`.
package goresilience

import (
	"context"

	"github.com/slok/goresilience/errors"
)

// Func is the function to execute with resilience.
type Func func(ctx context.Context) error

// command is the unit of execution.
type command struct{}

// Run satisfies Runner interface.
func (command) Run(ctx context.Context, f Func) error {
	// Only execute if we reached to the execution and the context has not been cancelled.
	select {
	case <-ctx.Done():
		return errors.ErrContextCanceled
	default:
		return f(ctx)
	}
}

// Runner knows how to execute a execution logic and returns error if errors.
type Runner interface {
	// Run will run the unit of execution passed on f.
	Run(ctx context.Context, f Func) error
}

// RunnerFunc is a helper that will satisfies circuit.Breaker interface by using a function.
type RunnerFunc func(ctx context.Context, f Func) error

// Run satisfies Runner interface.
func (r RunnerFunc) Run(ctx context.Context, f Func) error {
	return r(ctx, f)
}

// Middleware represents a middleware for a runner, it takes a runner and returns a runner.
type Middleware func(Runner) Runner

// RunnerChain will get N middleares and will create a Runner chain with them
// in the order that have been passed.
func RunnerChain(middlewares ...Middleware) Runner {
	// The bottom one is is the one that knows how to execute the command.
	var runner Runner = &command{}

	// Start wrapping in reverse order.
	for i := len(middlewares) - 1; i >= 0; i-- {
		runner = middlewares[i](runner)
	}

	// Return the chain.
	return runner
}

// SanitizeRunner returns a safe execution Runner if the runner is nil.
// Usually this helper will be used for the last part of the runner chain
// when the runner is nil, so instead of acting on a nil Runner its executed
// on a `command` Runner, this runner knows how to execute the `Func` function.
// It's safe to use it always as if it encounters a safe Runner it will return
// that Runner.
func SanitizeRunner(r Runner) Runner {
	// In case of end of execution chain.
	if r == nil {
		return &command{}
	}
	return r
}

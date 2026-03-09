package util

import (
	"context"
	"fmt"
	"time"
)

type result[T any] struct {
	value T
	err   error
}

// callbackWithContext is a function type that accepts a context and returns a result with an error.
type callbackWithContext[T any] func(ctx context.Context) (T, error)

// RunWithTimeoutAndResult executes a function with a timeout and returns both a result and an error.
// It returns the result and error from the function, or a nil result with timeout error if timeout occurs.
// Also, we can pass a context to the function for cancellation support.
func RunWithTimeoutAndResult[T any](timeout time.Duration, fn callbackWithContext[T]) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan result[T], 1)
	go func() {
		value, err := fn(ctx)
		done <- result[T]{value: value, err: err}
	}()

	select {
	case res := <-done:
		return res.value, res.err
	case <-ctx.Done():
		var zero T
		return zero, fmt.Errorf("timeout after %v", timeout)
	}
}

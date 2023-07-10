package timeout

import (
	"context"
	"time"

	"github.com/slok/goresilience"
	"github.com/slok/goresilience/errors"
	"github.com/slok/goresilience/metrics"
)

const (
	defaultTimeout = 1 * time.Second
)

// Config is the configuration of the timeout.
type Config struct {
	// Timeout is the duration that will be waited before giving as a timeouted execution.
	Timeout time.Duration
}

func (c *Config) defaults() {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
}

// result is a internal type used to send circuit breaker results
// using channels.
type result struct {
	fallback bool
	err      error
}

// New will wrap a execution unit that will cut the execution of
// a runner when some time passes using the context.
// use 0 timeout for default timeout.
func New(cfg Config) goresilience.Runner {
	return NewMiddleware(cfg)(nil)
}

// NewMiddleware returns a middleware that will cut the execution of
// a runner when some time passes using the context.
// use 0 timeout for default timeout.
func NewMiddleware(cfg Config) goresilience.Middleware {
	cfg.defaults()

	return func(next goresilience.Runner) goresilience.Runner {
		next = goresilience.SanitizeRunner(next)
		return goresilience.RunnerFunc(func(ctx context.Context, f goresilience.Func) error {
			metricsRecorder, _ := metrics.RecorderFromContext(ctx)

			// Set a timeout to the command using the context.
			// Should we cancel the context if finished...? I guess not, it could continue
			// the middleware chain.
			ctx, _ = context.WithTimeout(ctx, cfg.Timeout)

			// Run the command
			errc := make(chan error)
			go func() {
				errc <- next.Run(ctx, f)
			}()

			// Wait until the deadline has been reached or we have a result.
			select {
			// Finished correctly.
			case err := <-errc:
				return err
			// Timeout.
			case <-ctx.Done():
				metricsRecorder.IncTimeout()
				return errors.ErrTimeout
			}
		})
	}
}

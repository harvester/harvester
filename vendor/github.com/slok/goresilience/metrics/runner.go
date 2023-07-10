package metrics

import (
	"context"
	"time"

	"github.com/slok/goresilience"
)

var ctxRecorderKey contextKey = "recorder"

type contextKey string

func (c contextKey) String() string {
	return "metrics-ctx-key" + string(c)
}

// RecorderFromContext will get the metrics recorder from the context.
// If there is not context it will return also a dummy recorder that is
// safe to use it.
func RecorderFromContext(ctx context.Context) (recorder Recorder, ok bool) {
	rec, ok := ctx.Value(ctxRecorderKey).(Recorder)

	if !ok {
		return Dummy, false
	}

	return rec, true
}

func setRecorderOnContext(ctx context.Context, r Recorder) context.Context {
	return context.WithValue(ctx, ctxRecorderKey, r)
}

// NewMiddleware returns a middleware that will decorate a runner and measure
// the runner chain itself by setting the recorder on the context so the following
// runners can get and measure.
func NewMiddleware(id string, rec Recorder) goresilience.Middleware {
	if rec == nil {
		rec = Dummy
	}
	rec = rec.WithID(id)

	return func(next goresilience.Runner) goresilience.Runner {
		return goresilience.RunnerFunc(func(ctx context.Context, f goresilience.Func) (err error) {
			defer func(start time.Time) {
				rec.ObserveCommandExecution(start, err == nil)
			}(time.Now())

			// Set the recorder.
			// WARNING: This could have a performance impact due to the usage of reflect package
			// by the context. Measure if this has a big impact.
			ctx = setRecorderOnContext(ctx, rec)

			next = goresilience.SanitizeRunner(next)
			err = next.Run(ctx, f)

			return err
		})
	}
}

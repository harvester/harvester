package ticker

import (
	"context"
	"time"
)

func Context(ctx context.Context, duration time.Duration) <-chan time.Time {
	ticker := time.NewTicker(duration)
	c := make(chan time.Time)
	go func() {
		for {
			select {
			case t := <-ticker.C:
				c <- t
			case <-ctx.Done():
				close(c)
				ticker.Stop()
				return
			}
		}
	}()
	return c
}

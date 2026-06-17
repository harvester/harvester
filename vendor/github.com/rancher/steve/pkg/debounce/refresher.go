package debounce

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Refreshable represents an object which can be refreshed. This should be protected by a mutex for concurrent operation.
type Refreshable interface {
	Refresh() error
}

// DebounceableRefresher is used to debounce multiple attempts to refresh a refreshable type.
type DebounceableRefresher struct {
	// timerMu protects the scheduled timer.
	timerMu   sync.Mutex
	scheduled *time.Timer

	// A buffered channel of size 1 acts as the trigger. Selective sends (skipped when the buffer is full) allows deduplication
	trigger chan<- struct{}
}

func NewDebounceableRefresher(ctx context.Context, refreshable Refreshable, duration time.Duration) *DebounceableRefresher {
	triggerCh := make(chan struct{}, 1)
	dr := &DebounceableRefresher{
		trigger: triggerCh,
	}

	// Refresh loop
	go func() {
		for range triggerCh {
			if err := refreshable.Refresh(); err != nil {
				logrus.Errorf("failed to refresh with error: %v", err)
			}
		}
	}()

	// Cancellation and scheduling goroutine
	go func() {
		defer close(triggerCh)
		// Allow disabling ticker if a negative or zero duration is provided
		var tick <-chan time.Time
		if duration > 0 {
			ticker := time.NewTicker(duration)
			defer ticker.Stop()
			tick = ticker.C
		}
		for {
			select {
			case <-tick:
				dr.triggerRefresh()
			case <-ctx.Done():
				dr.trigger = nil
				return
			}
		}
	}()

	return dr
}

func (dr *DebounceableRefresher) triggerRefresh() {
	select {
	case dr.trigger <- struct{}{}:
	default: // Channel buffer is closed or full (a refresh is already pending)
	}
}

func (dr *DebounceableRefresher) Schedule(after time.Duration) {
	dr.timerMu.Lock()
	defer dr.timerMu.Unlock()

	// Stop any previously scheduled timer.
	if dr.scheduled != nil {
		dr.scheduled.Stop()
	}

	// Schedule a new timer that will trigger a refresh.
	dr.scheduled = time.AfterFunc(after, dr.triggerRefresh)
}

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
	sync.Mutex
	// Refreshable is any type that can be refreshed. The refresh method should by protected by a mutex internally.
	Refreshable Refreshable
	current     context.CancelFunc
	onCancel    func()
}

// RefreshAfter requests a refresh after a certain time has passed. Subsequent calls to this method will
// delay the requested refresh by the new duration. Note that this is a total override of the previous calls - calling
// RefreshAfter(time.Second * 2) and then immediately calling RefreshAfter(time.Microsecond * 1) will run a refresh
// in one microsecond
func (d *DebounceableRefresher) RefreshAfter(duration time.Duration) {
	d.Lock()
	defer d.Unlock()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	if d.current != nil {
		d.current()
	}
	d.current = cancel
	go func() {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			// this indicates that the context was cancelled.
			if d.onCancel != nil {
				d.onCancel()
			}
		case <-timer.C:
			// note this can cause multiple refreshes to happen concurrently
			err := d.Refreshable.Refresh()
			if err != nil {
				logrus.Errorf("failed to refresh with error: %v", err)
			}
		}
	}()
}

package util

import (
	"sync"
	"time"
)

// SharedTimeouts has the following use case:
// - Multiple goroutines may need to time out eventually.
// - Only the goroutines themselves know if the conditions for a timeout have been met.
// - It is fine for some of the goroutines to time out quickly.
// - The last goroutine should time out more slowly.
// SharedTimeouts implements the types.SharedTimeouts instead of directly defining the concrete type to avoid an import
// loop.
type SharedTimeouts struct {
	mutex        sync.RWMutex
	longTimeout  time.Duration
	shortTimeout time.Duration
	numConsumers int
}

func NewSharedTimeouts(shortTimeout, longTimeout time.Duration) *SharedTimeouts {
	return &SharedTimeouts{
		longTimeout:  longTimeout,
		shortTimeout: shortTimeout,
	}
}

func (t *SharedTimeouts) Increment() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.numConsumers++
}

func (t *SharedTimeouts) Decrement() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.numConsumers--
}

// CheckAndDecrement checks if duration exceeds longTimeout or shortTimeout, returns the timeout exceeded (if
// applicable) and decrements numConsumers.
// - shortTimeout is only considered exceeded if there is still one other consumer to wait for longTimeout.
// - The caller MUST take whatever action is required for a timeout if a value > 0 is returned.
func (t *SharedTimeouts) CheckAndDecrement(duration time.Duration) time.Duration {
	if duration > t.longTimeout {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		t.numConsumers--
		return t.longTimeout
	}

	if duration > t.shortTimeout {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		if t.numConsumers > 1 {
			t.numConsumers--
			return t.shortTimeout
		}
	}

	return 0
}

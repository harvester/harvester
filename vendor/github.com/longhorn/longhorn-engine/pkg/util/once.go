package util

import (
	"sync"
	"sync/atomic"
)

// Once is modification of sync.Once to only set done once Do has run f() once successfully
// A Once must not be copied after first use.
type Once struct {
	// done is set only when Do has run f() one time successfully
	done uint32
	m    sync.Mutex
}

// Do execute f() only if it has never been run successfully before
// if f() encounters an error it will be returned to the caller
func (o *Once) Do(f func() error) error {
	if atomic.LoadUint32(&o.done) == 1 { //fast path
		return nil
	}

	return o.doSlow(f)
}

func (o *Once) doSlow(f func() error) error {
	o.m.Lock()
	defer o.m.Unlock()

	var err error
	if o.done == 0 {
		if err = f(); err == nil {
			defer atomic.StoreUint32(&o.done, 1)
		}
	}
	return err
}

package remotedialer

import (
	"sync"
)

type backPressure struct {
	cond   sync.Cond
	c      *connection
	paused bool
}

func newBackPressure(c *connection) *backPressure {
	return &backPressure{
		cond: sync.Cond{
			L: &sync.Mutex{},
		},
		c:      c,
		paused: false,
	}
}

func (b *backPressure) OnPause() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	b.paused = true
	b.cond.Broadcast()
}

func (b *backPressure) OnResume() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	b.paused = false
	b.cond.Broadcast()
}

func (b *backPressure) Pause() error {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()
	if b.paused {
		return nil
	}
	if _, err := b.c.Pause(); err != nil {
		return err
	}
	b.paused = true
	return nil
}

func (b *backPressure) Resume() error {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()
	if !b.paused {
		return nil
	}
	if _, err := b.c.Resume(); err != nil {
		return err
	}
	b.paused = false
	return nil
}

func (b *backPressure) Wait() {
	b.cond.L.Lock()
	defer b.cond.L.Unlock()

	for b.paused {
		b.cond.Wait()
	}
}

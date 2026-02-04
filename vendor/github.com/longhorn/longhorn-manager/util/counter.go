package util

import (
	"sync/atomic"
)

type AtomicCounter struct {
	count int32
}

type Counter interface {
	GetCount() int32
	IncreaseCount()
	DecreaseCount()
	ResetCount()
}

func NewAtomicCounter() Counter {
	return &AtomicCounter{
		count: 0,
	}
}

func (ac *AtomicCounter) GetCount() int32 {
	return atomic.LoadInt32(&ac.count)
}

func (ac *AtomicCounter) IncreaseCount() {
	atomic.AddInt32(&ac.count, 1)
}

func (ac *AtomicCounter) DecreaseCount() {
	atomic.AddInt32(&ac.count, -1)
}

func (ac *AtomicCounter) ResetCount() {
	atomic.StoreInt32(&ac.count, 0)
}

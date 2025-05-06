package util

import (
	"sync"
	"time"
)

type counterEntry struct {
	count      int
	lastUpdate time.Time
}

type TimedCounter struct {
	sync.RWMutex
	perItemCounter map[string]*counterEntry

	expiredDuration time.Duration
}

func NewTimedCounter(expiredDuration time.Duration) *TimedCounter {
	return &TimedCounter{
		perItemCounter:  map[string]*counterEntry{},
		expiredDuration: expiredDuration,
	}
}

func (c *TimedCounter) GetCount(id string) int {
	c.RLock()
	defer c.RUnlock()
	entry, ok := c.perItemCounter[id]
	if ok {
		return entry.count
	}
	return 0
}

func (c *TimedCounter) IncreaseCount(id string) {
	c.Lock()
	defer c.Unlock()
	entry, ok := c.perItemCounter[id]
	if !ok {
		entry = c.initEntryUnsafe(id)
	}
	entry.count += 1
	entry.lastUpdate = time.Now()
}

func (c *TimedCounter) DeleteEntry(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.perItemCounter, id)
}

func (c *TimedCounter) gc() {
	c.Lock()
	defer c.Unlock()
	now := time.Now()
	for id, entry := range c.perItemCounter {
		if now.Sub(entry.lastUpdate) > c.expiredDuration {
			delete(c.perItemCounter, id)
		}
	}
}

func (c *TimedCounter) RunGC(gcDuration time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(gcDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.gc()
		case <-stopCh:
			return
		}
	}
}

func (c *TimedCounter) GetTotalEntries() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.perItemCounter)
}

func (c *TimedCounter) initEntryUnsafe(id string) *counterEntry {
	entry := &counterEntry{count: 0}
	c.perItemCounter[id] = entry
	return entry
}

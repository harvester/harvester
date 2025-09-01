package subscribe

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/apiserver/pkg/types"
)

type DebouncerState int

const (
	// The first notification is always sent right away, no need to wait
	FirstNotification DebouncerState = iota
	TimerStarted
	TimerStopped
)

type debouncer struct {
	lock sync.Mutex

	timer        *time.Timer
	debounceRate time.Duration

	inCh  chan types.APIEvent
	outCh chan types.APIEvent
}

func newDebouncer(debounceRate time.Duration, eventsCh chan types.APIEvent) *debouncer {
	d := &debouncer{
		debounceRate: debounceRate,
		timer:        time.NewTimer(debounceRate),
		inCh:         eventsCh,
		outCh:        make(chan types.APIEvent),
	}
	d.timer.Stop()
	return d
}

func (d *debouncer) Run(ctx context.Context) {
	state := FirstNotification
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case ev, ok := <-d.inCh:
			if ev.Error != nil {
				ev.Name = string(SubscriptionModeNotification)
				d.outCh <- ev
				break loop
			}

			if !ok {
				break loop
			}

			d.lock.Lock()
			switch state {
			case FirstNotification:
				d.outCh <- types.APIEvent{
					Name: string(SubscriptionModeNotification),
				}
				state = TimerStopped
			case TimerStopped:
				state = TimerStarted
				d.timer.Reset(d.debounceRate)
			}
			d.lock.Unlock()
		case <-d.timer.C:
			d.lock.Lock()
			d.outCh <- types.APIEvent{
				Name: string(SubscriptionModeNotification),
			}
			d.timer.Stop()
			state = TimerStopped
			d.lock.Unlock()
		}
	}

	close(d.outCh)
}

func (d *debouncer) NotificationsChan() chan types.APIEvent {
	return d.outCh
}

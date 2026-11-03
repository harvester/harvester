package subscribe

import (
	"context"
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
	defer close(d.outCh)

	var latestRV string
	state := FirstNotification
	for {
		select {
		case <-ctx.Done():
			// Despite context being closed, respect the original channel state, only closing outCh when inCh is closed
			for range d.inCh {
			}
			return
		case ev, ok := <-d.inCh:
			if !ok {
				return
			}
			if ev.Error != nil {
				ev.Name = string(SubscriptionModeNotification)
				d.outCh <- ev
				return
			}

			latestRV = ev.Revision
			switch state {
			case FirstNotification:
				d.outCh <- types.APIEvent{
					Name:     string(SubscriptionModeNotification),
					Revision: ev.Revision,
				}
				state = TimerStopped
			case TimerStopped:
				state = TimerStarted
				d.timer.Reset(d.debounceRate)
			}
		case <-d.timer.C:
			d.outCh <- types.APIEvent{
				Name:     string(SubscriptionModeNotification),
				Revision: latestRV,
			}
			state = TimerStopped
			d.timer.Stop()
		}
	}
}

func (d *debouncer) NotificationsChan() chan types.APIEvent {
	return d.outCh
}

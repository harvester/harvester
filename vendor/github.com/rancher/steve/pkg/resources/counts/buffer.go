package counts

import (
	"time"

	"github.com/rancher/apiserver/pkg/types"
)

// debounceDuration determines how long events will be held before they are sent to the consumer
var debounceDuration = 5 * time.Second

// countsBuffer creates an APIEvent channel with a buffered response time (i.e. replies are only sent once every second)
func countsBuffer(c chan Count) chan types.APIEvent {
	result := make(chan types.APIEvent)
	go func() {
		defer close(result)
		debounceCounts(result, c)
	}()
	return result
}

// debounceCounts converts counts from an input channel into an APIEvent, and updates the result channel at a reduced pace
func debounceCounts(result chan types.APIEvent, input chan Count) {
	// counts aren't a critical value. To avoid excess UI processing, only send updates after debounceDuration has elapsed
	t := time.NewTicker(debounceDuration)
	defer t.Stop()

	var currentCount *Count

	firstCount, fOk := <-input
	if fOk {
		// send a count immediately or we will have to wait a second for the first update
		result <- toAPIEvent(firstCount)
	}
	for {
		select {
		case count, ok := <-input:
			if !ok {
				return
			}
			if currentCount == nil {
				currentCount = &count
			} else {
				itemCounts := count.Counts
				for id, itemCount := range itemCounts {
					// our current count will be outdated in comparison with anything in the new events
					currentCount.Counts[id] = itemCount
				}
			}
		case <-t.C:
			if currentCount != nil {
				result <- toAPIEvent(*currentCount)
				currentCount = nil
			}
		}
	}
}

func toAPIEvent(count Count) types.APIEvent {
	return types.APIEvent{
		Name:         "resource.change",
		ResourceType: "counts",
		Object:       toAPIObject(count),
	}
}

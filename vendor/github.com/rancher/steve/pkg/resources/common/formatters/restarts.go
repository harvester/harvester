package formatters

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/duration"
)

// FormatRestarts formats a restart count array [count, timestamp_ms] as a display string.
// Examples: "0", "4 (3h38m ago)"
func FormatRestarts(values []interface{}) string {
	primary := toInt64(values, 0)
	secondary := toInt64(values, 1)

	if secondary == 0 {
		return fmt.Sprintf("%d", primary)
	}

	// Calculate fresh duration from timestamp
	timestamp := time.UnixMilli(secondary)
	dur := time.Since(timestamp)
	humanDur := duration.HumanDuration(dur)

	return fmt.Sprintf("%d (%s ago)", primary, humanDur)
}

// toInt64 safely extracts an int64 from a slice at the given index
func toInt64(arr []interface{}, idx int) int64 {
	if idx >= len(arr) || arr[idx] == nil {
		return 0
	}
	switch v := arr[idx].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

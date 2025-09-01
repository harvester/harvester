package common

import (
	"fmt"
	"strings"
	"time"
)

// ParseTimestampOrHumanReadableDuration can do one of three things with an incoming string:
// 1. Recognize it's an absolute timestamp and calculate a relative `time.Duration`
// 2. Recognize it's a human-readable duration (like 3m) and convert to a relative `time.Duration`
// 3. Return an error because it doesn't recognize the input
func ParseTimestampOrHumanReadableDuration(s string) (time.Duration, error) {
	var total time.Duration
	var val int
	var unit byte

	parsedTime, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return time.Since(parsedTime), nil
	}

	r := strings.NewReader(s)
	for r.Len() > 0 {
		if _, err := fmt.Fscanf(r, "%d%c", &val, &unit); err != nil {
			return 0, fmt.Errorf("invalid duration in %s: %w", s, err)
		}

		switch unit {
		case 'd':
			total += time.Duration(val) * 24 * time.Hour
		case 'h':
			total += time.Duration(val) * time.Hour
		case 'm':
			total += time.Duration(val) * time.Minute
		case 's':
			total += time.Duration(val) * time.Second
		default:
			return 0, fmt.Errorf("invalid duration unit %s in %s", string(unit), s)
		}
	}

	return total, nil
}

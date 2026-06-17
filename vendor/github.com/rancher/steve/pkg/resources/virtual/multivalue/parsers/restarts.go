package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	rescommon "github.com/rancher/steve/pkg/resources/common"
	"github.com/sirupsen/logrus"
)

var Now = time.Now

var restartsPattern = regexp.MustCompile(`^(\d+)(?:\s+\((.+?)\s+ago\))?$`)

// ParseRestarts parses pod restart values like "4 (3h38m ago)" into [count, timestamp_ms]
func ParseRestarts(value string) ([]interface{}, error) {
	matches := restartsPattern.FindStringSubmatch(value)
	if matches == nil {
		return nil, fmt.Errorf("invalid restarts format: %q", value)
	}

	count, _ := strconv.ParseInt(matches[1], 10, 64)

	var timestamp interface{}
	if matches[2] != "" {
		dur, err := rescommon.ParseHumanReadableDuration(matches[2])
		if err != nil {
			logrus.Errorf("failed to parse restart duration %q: %v", matches[2], err)
		} else {
			timestamp = Now().Add(-dur).UnixMilli()
		}
	}

	return []interface{}{count, timestamp}, nil
}

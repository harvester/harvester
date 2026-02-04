package timewindow

import (
	"fmt"
	"time"
)

// TimeWindow specifies a schedule of days and times.
type TimeWindow struct {
	days      weekdays
	location  *time.Location
	startTime time.Time
	endTime   time.Time
}

// New creates a TimeWindow instance based on string inputs specifying a schedule.
func New(days []string, startTime, endTime, location string) (*TimeWindow, error) {
	tw := &TimeWindow{}

	var err error
	if tw.days, err = parseWeekdays(days); err != nil {
		return nil, err
	}

	if tw.location, err = time.LoadLocation(location); err != nil {
		return nil, err
	}

	if tw.startTime, err = parseTime(startTime, tw.location); err != nil {
		return nil, err
	}

	if tw.endTime, err = parseTime(endTime, tw.location); err != nil {
		return nil, err
	}

	return tw, nil
}

// Contains determines whether the specified time is within this time window.
func (tw *TimeWindow) Contains(t time.Time) bool {
	loctime := t.In(tw.location)
	if !tw.days.Contains(loctime.Weekday()) {
		return false
	}

	start := time.Date(loctime.Year(), loctime.Month(), loctime.Day(), tw.startTime.Hour(), tw.startTime.Minute(), tw.startTime.Second(), 0, tw.location)
	end := time.Date(loctime.Year(), loctime.Month(), loctime.Day(), tw.endTime.Hour(), tw.endTime.Minute(), tw.endTime.Second(), 1e9-1, tw.location)

	// Time Wrap validation
	// First we check for start and end time, if start is after end time
	// Next we need to validate if we want to wrap to the day before or to the day after
	// For that we check the loctime value to see if it is before end time, we wrap with the day before
	// Otherwise we wrap to the next day.
	if tw.startTime.After(tw.endTime) {
		if loctime.Before(end) {
			start = start.Add(-24 * time.Hour)
		} else {
			end = end.Add(24 * time.Hour)
		}
	}

	return (loctime.After(start) || loctime.Equal(start)) && (loctime.Before(end) || loctime.Equal(end))
}

// String returns a string representation of this time window.
func (tw *TimeWindow) String() string {
	return fmt.Sprintf("%s between %02d:%02d and %02d:%02d %s", tw.days.String(), tw.startTime.Hour(), tw.startTime.Minute(), tw.endTime.Hour(), tw.endTime.Minute(), tw.location.String())
}

// parseTime tries to parse a time with several formats.
func parseTime(s string, loc *time.Location) (time.Time, error) {
	fmts := []string{"15:04", "15:04:05", "03:04pm", "15", "03pm", "3pm"}
	for _, f := range fmts {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, nil
		}
	}

	return time.Now(), fmt.Errorf("Invalid time format: %s", s)
}

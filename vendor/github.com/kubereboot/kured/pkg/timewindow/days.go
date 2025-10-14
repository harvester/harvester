package timewindow

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EveryDay contains all days of the week, and exports it
// for convenience use in the cmd line arguments.
var EveryDay = []string{"su", "mo", "tu", "we", "th", "fr", "sa"}

// dayStrings maps day strings to time.Weekdays
var dayStrings = map[string]time.Weekday{
	"su":        time.Sunday,
	"sun":       time.Sunday,
	"sunday":    time.Sunday,
	"mo":        time.Monday,
	"mon":       time.Monday,
	"monday":    time.Monday,
	"tu":        time.Tuesday,
	"tue":       time.Tuesday,
	"tuesday":   time.Tuesday,
	"we":        time.Wednesday,
	"wed":       time.Wednesday,
	"wednesday": time.Wednesday,
	"th":        time.Thursday,
	"thu":       time.Thursday,
	"thursday":  time.Thursday,
	"fr":        time.Friday,
	"fri":       time.Friday,
	"friday":    time.Friday,
	"sa":        time.Saturday,
	"sat":       time.Saturday,
	"saturday":  time.Saturday,
}

type weekdays uint32

// parseWeekdays creates a set of weekdays from a string slice
func parseWeekdays(days []string) (weekdays, error) {
	var result uint32
	for _, day := range days {
		if len(day) == 0 {
			continue
		}

		weekday, err := parseWeekday(day)
		if err != nil {
			return weekdays(0), err
		}

		result |= 1 << uint32(weekday)
	}

	return weekdays(result), nil
}

// Contains returns true if the specified weekday is a member of this set.
func (w weekdays) Contains(day time.Weekday) bool {
	return uint32(w)&(1<<uint32(day)) != 0
}

// String returns a string representation of the set of weekdays.
func (w weekdays) String() string {
	var b strings.Builder
	for i := uint32(0); i < 7; i++ {
		if uint32(w)&(1<<i) != 0 {
			b.WriteString(time.Weekday(i).String()[0:3])
		} else {
			b.WriteString("---")
		}
	}

	return b.String()
}

func parseWeekday(day string) (time.Weekday, error) {
	if n, err := strconv.Atoi(day); err == nil {
		if n >= 0 && n < 7 {
			return time.Weekday(n), nil
		}
		return time.Sunday, fmt.Errorf("Invalid weekday, number out of range: %s", day)
	}

	if weekday, ok := dayStrings[strings.ToLower(day)]; ok {
		return weekday, nil
	}
	return time.Sunday, fmt.Errorf("Invalid weekday: %s", day)
}

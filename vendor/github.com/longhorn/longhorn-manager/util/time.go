package util

import (
	"fmt"
	"strings"
	"time"
)

func ParseTimeZ(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	return t, err
}

func ParseLocalTime(ts string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(time.RFC3339, ts, loc)
	if e, ok := err.(*time.ParseError); ok && e.LayoutElem == "Z07:00" && e.ValueElem == "" {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", ts, loc)
	}
	return t, err
}

func FormatTimeZ(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func FormatLocalTime(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(time.RFC3339)
}

func FromMillis(t int64) time.Time {
	return time.Unix(t/1000, t%1000*1000000)
}

func LimitToPeriod(p, t [2]time.Time) [2]time.Time {
	if t[0].Before(p[0]) || t[0].IsZero() {
		t[0] = p[0]
	}
	if t[1].After(p[1]) || t[1].IsZero() {
		t[1] = p[1]
	}
	return t
}

func ParsePeriod(s string, loc *time.Location) ([2]time.Time, error) {
	var r [2]time.Time

	if s == "" {
		// default: from Unix epoch to now
		r[0] = time.Unix(0, 0).UTC()
		r[1] = time.Now().UTC()
		return r, nil
	}

	ts := strings.Split(s, "/")

	var err error
	switch len(ts) {
	case 1:
		if r[0], err = ParseLocalTime(s, loc); err != nil {
			return r, err
		}
		r[1] = time.Now().In(loc)
		return r, nil
	case 2:
		if r[0], err = ParseLocalTime(ts[0], loc); err != nil {
			return r, err
		}
		if strings.HasPrefix(ts[1], "P") {
			// TODO parse duration
		} else {
			if r[1], err = ParseLocalTime(ts[1], loc); err != nil {
				return r, err
			}
			return r, nil
		}
	}

	return r, fmt.Errorf("error parsing time interval '%s'", s)
}

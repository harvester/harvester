package logging

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

const slowQueryThreshold = 50 * time.Millisecond

type QueryLogEntry struct {
	Timestamp    time.Time `json:"ts"`
	Query        string    `json:"query"`
	Params       []any     `json:"params"`
	DurationNano int64     `json:"duration_nano"`
}

type QueryLogger interface {
	Log(startTime time.Time, query string, params []any)
}

type NoopQueryLogger struct{}

func (NoopQueryLogger) Log(_ time.Time, _ string, _ []any) {}

type logger chan QueryLogEntry

func (l logger) Log(start time.Time, query string, params []any) {
	elapsed := time.Since(start)
	l <- QueryLogEntry{
		Timestamp:    time.Now(),
		DurationNano: elapsed.Nanoseconds(),
		Query:        query,
		Params:       params,
	}

	if elapsed > slowQueryThreshold {
		logrus.Warnf("Query took %s (threshold %s): %q, params %q\n", elapsed, slowQueryThreshold, query, params)
	}
}

func StartQueryLogger(ctx context.Context, p string, includeParams bool) (QueryLogger, error) {
	if p == "" {
		return &NoopQueryLogger{}, nil
	}

	c := make(logger, 1)
	f, err := os.Create(p)
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			// In case there was an error, stop writing and avoid blocking senders
			for range c {
			}
		}()
		defer f.Close()
		enc := json.NewEncoder(f)
		for entry := range c {
			if !includeParams {
				entry.Params = nil
			}
			if err := enc.Encode(entry); err != nil {
				logrus.WithError(err).Errorln("error writing query log")
				return
			}
		}
	}()
	go func() {
		<-ctx.Done()
		close(c)
	}()
	return c, nil
}

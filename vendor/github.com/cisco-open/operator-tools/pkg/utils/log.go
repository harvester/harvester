// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"fmt"
	"io"
	"os"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	"github.com/spf13/cast"
)

var GlobalLogLevel = 0

var Log = NewLogger("", os.Stderr, os.Stderr, 0)

func NewLogger(name string, out, err io.Writer, level int) logr.Logger {
	sink := &logSink{
		name:   name,
		values: make([]interface{}, 0),
		out:    out,
		err:    err,
	}
	return logr.New(sink).V(level)
}

func joinAndSeparatePairs(values []interface{}) string {
	joined := ""
	for i, v := range values {
		joined += cast.ToString(v)
		if i%2 == 0 {
			joined += ": "
		} else {
			if i < len(values)-1 {
				joined += ", "
			}
		}
	}
	return joined
}

func getDetailedErr(err error) string {
	details := errors.GetDetails(err)
	if len(details) == 0 {
		return err.Error()
	}
	return fmt.Sprintf("%s (%s)", err.Error(), joinAndSeparatePairs(details))
}

type logSink struct {
	name   string
	values []interface{}
	out    io.Writer
	err    io.Writer
}

func (s logSink) Init(logr.RuntimeInfo) {}

func (s logSink) Enabled(level int) bool {
	return GlobalLogLevel >= level
}

func (s logSink) Info(level int, msg string, keysAndValues ...interface{}) {
	if !s.Enabled(level) {
		return
	}
	s.values = append(s.values, keysAndValues...)
	if len(s.values) == 0 {
		_, _ = fmt.Fprintf(s.out, "%s> %s\n", s.name, msg)
	} else {
		_, _ = fmt.Fprintf(s.out, "%s> %s %s\n", s.name, msg, joinAndSeparatePairs(s.values))
	}
}

func (s logSink) Error(err error, msg string, keysAndValues ...interface{}) {
	s.values = append(s.values, keysAndValues...)
	if len(s.values) == 0 {
		_, _ = fmt.Fprintf(s.err, "%s> %s %s\n", s.name, msg, getDetailedErr(err))
	} else {
		_, _ = fmt.Fprintf(s.err, "%s> %s %s %s\n", s.name, msg, getDetailedErr(err), joinAndSeparatePairs(s.values))
	}
}

func (s logSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	l := len(s.values)
	s.values = append(s.values[:l:l], keysAndValues...)
	return s
}

func (s logSink) WithName(name string) logr.LogSink {
	s.name += name
	return s
}

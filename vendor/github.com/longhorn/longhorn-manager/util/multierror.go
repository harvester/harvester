package util

import (
	"sort"
	"strings"
)

type MultiError map[string]struct{}

func NewMultiError(errs ...string) MultiError {
	multiError := MultiError{}
	for _, err := range errs {
		multiError[err] = struct{}{}
	}

	return multiError
}

func (me MultiError) Append(errs MultiError) {
	for err := range errs {
		me[err] = struct{}{}
	}
}

func (me MultiError) Join() string {
	keys := make([]string, 0, len(me))
	for err := range me {
		keys = append(keys, err)
	}

	sort.Strings(keys)

	return strings.Join(keys, ";")
}

func (me MultiError) Reset() {
	for k := range me {
		delete(me, k)
	}
}

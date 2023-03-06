package internal

import (
	"errors"
	"fmt"
)

// NoResponderFound is returned when no responders are found for a
// given HTTP method and URL.
var NoResponderFound = errors.New("no responder found") // nolint: revive

// ErrorNoResponderFoundMistake encapsulates a NoResponderFound
// error probably due to a user error on the method or URL path.
type ErrorNoResponderFoundMistake struct {
	Kind      string // "method", "URL" or "matcher"
	Orig      string // original wrong method/URL, without any matching responder
	Suggested string // suggested method/URL with a matching responder
}

var _ error = (*ErrorNoResponderFoundMistake)(nil)

// Unwrap implements the interface needed by errors.Unwrap.
func (e *ErrorNoResponderFoundMistake) Unwrap() error {
	return NoResponderFound
}

// Error implements error interface.
func (e *ErrorNoResponderFoundMistake) Error() string {
	if e.Kind == "matcher" {
		return fmt.Sprintf("%s despite %s",
			NoResponderFound,
			e.Suggested,
		)
	}
	return fmt.Sprintf("%[1]s for %[2]s %[3]q, but one matches %[2]s %[4]q",
		NoResponderFound,
		e.Kind,
		e.Orig,
		e.Suggested,
	)
}

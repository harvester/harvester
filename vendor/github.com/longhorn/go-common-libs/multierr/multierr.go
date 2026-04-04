package multierr

import (
	"sort"
	"strings"
)

type MultiError map[string][]error

func NewMultiError() MultiError {
	return make(MultiError)
}

func (me MultiError) Append(reason string, err error) {
	if reason != "" && err != nil {
		me[reason] = append(me[reason], err)
	}
}

func (me MultiError) AppendMultiError(other MultiError) {
	for reason, errs := range other {
		for _, err := range errs {
			me.Append(reason, err)
		}
	}
}

func (me MultiError) Reset() {
	clear(me) // Go 1.21+
}

func (me MultiError) JoinReasons() string {
	if len(me) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(me))
	for reason := range me {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return strings.Join(reasons, "; ")
}

func (me MultiError) Error() string {
	if len(me) == 0 {
		return ""
	}
	var sb strings.Builder
	for reason, errs := range me {
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(reason + ": ")

		for i, err := range errs {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(err.Error())
		}
	}
	return sb.String()
}

func (me MultiError) ErrorByReason(reason string) string {
	errs, exists := me[reason]
	if !exists || len(errs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, err := range errs {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

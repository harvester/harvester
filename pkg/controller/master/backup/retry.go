package backup

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// onlyRetry requires every cause to be a conflict or an expected pending state.
// Inspect joined errors before matching so one retry signal cannot hide a failure.
func onlyRetry(err error, pending ...error) bool {
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		causes := wrapped.Unwrap()
		for _, cause := range causes {
			if !onlyRetry(cause, pending...) {
				return false
			}
		}
		return len(causes) > 0
	case interface{ Unwrap() error }:
		return onlyRetry(wrapped.Unwrap(), pending...)
	}
	if apierrors.IsConflict(err) {
		return true
	}
	for _, state := range pending {
		if errors.Is(err, state) {
			return true
		}
	}
	return false
}

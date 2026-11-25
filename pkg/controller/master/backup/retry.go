package backup

import (
	"errors"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/harvester/harvester/pkg/util/datamover"
)

// onlyRetry requires every cause to be transient or an expected pending state.
// Inspect joined errors through wrappers before matching so a retry signal cannot
// hide a failure, while preserving typed errors such as net.OpError.
func onlyRetry(err error, pending ...error) bool {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		causes := joined.Unwrap()
		for _, cause := range causes {
			if !onlyRetry(cause, pending...) {
				return false
			}
		}
		return len(causes) > 0
	}
	if errors.Is(err, datamover.ErrPending) || isTransient(err) {
		return true
	}
	for _, state := range pending {
		if errors.Is(err, state) {
			return true
		}
	}
	return false
}

func isTransient(err error) bool {
	var networkError net.Error
	return apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) ||
		apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) || errors.As(err, &networkError)
}

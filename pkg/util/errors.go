package util

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	StatusReasonStillExists metav1.StatusReason = "StillExists"
)

func IsStillExists(err error) bool {
	return apierrors.ReasonForError(err) == StatusReasonStillExists
}

func NewStillExists(qualifiedResource schema.GroupResource, name string) *apierrors.StatusError {
	return &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusConflict,
			Reason: StatusReasonStillExists,
			Details: &metav1.StatusDetails{
				Group: qualifiedResource.Group,
				Kind:  qualifiedResource.Resource,
				Name:  name,
			},
			Message: fmt.Sprintf("%s %q still exists", qualifiedResource.String(), name),
		},
	}
}

func IsRetriableNetworkError(err error) bool {
	if os.IsTimeout(err) {
		return true
	} else if errors.Is(err, syscall.ENETDOWN) {
		return true
	} else if errors.Is(err, syscall.ENETUNREACH) {
		return true
	} else if errors.Is(err, syscall.ECONNRESET) {
		return true
	} else if errors.Is(err, syscall.ETIMEDOUT) {
		return true
	} else if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	} else if errors.Is(err, syscall.EHOSTUNREACH) {
		return true
	} else {
		return false
	}
}

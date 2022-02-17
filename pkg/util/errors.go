package util

import (
	"fmt"
	"net/http"

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

package error

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AdmitError struct {
	message string
	code    int32
	reason  metav1.StatusReason
	causes  []metav1.StatusCause
}

func (e AdmitError) Error() string {
	return e.message
}

func (e AdmitError) AsResult() *metav1.Status {
	status := metav1.Status{
		Status:  "Failure",
		Message: e.message,
		Code:    e.code,
		Reason:  e.reason,
	}

	if len(e.causes) > 0 {
		status.Details = &metav1.StatusDetails{
			Causes: e.causes,
		}
	}

	return &status
}

// 400
func NewBadRequest(message string) AdmitError {
	return AdmitError{
		code:    http.StatusBadRequest,
		message: message,
		reason:  metav1.StatusReasonBadRequest,
	}
}

// 405
func NewMethodNotAllowed(message string) AdmitError {
	return AdmitError{
		code:    http.StatusMethodNotAllowed,
		message: message,
		reason:  metav1.StatusReasonMethodNotAllowed,
	}
}

// 422
func NewInvalidError(message string, field string) AdmitError {
	return AdmitError{
		code:    http.StatusUnprocessableEntity,
		message: message,
		reason:  metav1.StatusReasonInvalid,
		causes: []metav1.StatusCause{
			{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: message,
				Field:   field,
			},
		},
	}
}

// 409
func NewConflict(message string) AdmitError {
	return AdmitError{
		code:    http.StatusConflict,
		message: message,
		reason:  metav1.StatusReasonConflict,
	}
}

// 500
func NewInternalError(message string) AdmitError {
	return AdmitError{
		code:    http.StatusInternalServerError,
		message: message,
		reason:  metav1.StatusReasonInternalError,
	}
}

// 500
// Let error package to unwrap the string
func NewInternalErrorFromErr(err error) AdmitError {
	return AdmitError{
		code:    http.StatusInternalServerError,
		message: err.Error(),
		reason:  metav1.StatusReasonInternalError,
	}
}

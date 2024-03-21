package resourcequota

import "fmt"

const (
	ErrInsufficientResourcesFMT = "%s insufficient resources due to resource quota"
)

type InsufficientResourceError struct {
	msg string
}

func (e *InsufficientResourceError) Error() string {
	return e.msg
}

func newInsufficientResourceError(t string) error {
	return &InsufficientResourceError{
		msg: fmt.Sprintf(ErrInsufficientResourcesFMT, t),
	}
}

func cpuInsufficientResourceError() error {
	return newInsufficientResourceError("cpu")
}

func memInsufficientResourceError() error {
	return newInsufficientResourceError("memory")
}

func storageInsufficientResourceError() error {
	return newInsufficientResourceError("storage")
}

func IsInsufficientResourceError(err error) bool {
	_, ok := err.(*InsufficientResourceError)
	return ok
}

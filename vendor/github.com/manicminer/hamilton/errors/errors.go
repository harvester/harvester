package errors

import "fmt"

// AlreadyExistsError is an error returned when an entity or object being created already exists.
type AlreadyExistsError struct {
	Obj string
	Id  string
}

// Error returns an error string for AlreadyExistsError.
func (e AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s with ID %q already exists", e.Obj, e.Id)
}

package ovsdb

import "fmt"

const (
	referentialIntegrityViolation = "referential integrity violation"
	constraintViolation           = "constraint violation"
	resourcesExhausted            = "resources exhausted"
	ioError                       = "I/O error"
	duplicateUUIDName             = "duplicate uuid name"
	domainError                   = "domain error"
	rangeError                    = "range error"
	timedOut                      = "timed out"
	notSupported                  = "not supported"
	aborted                       = "aborted"
	notOwner                      = "not owner"
)

// errorFromResult returns an specific OVSDB error type from
// an OperationResult
func errorFromResult(op *Operation, r OperationResult) OperationError {
	if r.Error == "" {
		return nil
	}
	switch r.Error {
	case referentialIntegrityViolation:
		return &ReferentialIntegrityViolation{r.Details, op}
	case constraintViolation:
		return &ConstraintViolation{r.Details, op}
	case resourcesExhausted:
		return &ResourcesExhausted{r.Details, op}
	case ioError:
		return &IOError{r.Details, op}
	case duplicateUUIDName:
		return &DuplicateUUIDName{r.Details, op}
	case domainError:
		return &DomainError{r.Details, op}
	case rangeError:
		return &RangeError{r.Details, op}
	case timedOut:
		return &TimedOut{r.Details, op}
	case notSupported:
		return &NotSupported{r.Details, op}
	case aborted:
		return &Aborted{r.Details, op}
	case notOwner:
		return &NotOwner{r.Details, op}
	default:
		return &Error{r.Error, r.Details, op}
	}
}

func ResultFromError(err error) OperationResult {
	if err == nil {
		panic("Program error: passed nil error to resultFromError")
	}
	switch e := err.(type) {
	case *ReferentialIntegrityViolation:
		return OperationResult{Error: referentialIntegrityViolation, Details: e.details}
	case *ConstraintViolation:
		return OperationResult{Error: constraintViolation, Details: e.details}
	case *ResourcesExhausted:
		return OperationResult{Error: resourcesExhausted, Details: e.details}
	case *IOError:
		return OperationResult{Error: ioError, Details: e.details}
	case *DuplicateUUIDName:
		return OperationResult{Error: duplicateUUIDName, Details: e.details}
	case *DomainError:
		return OperationResult{Error: domainError, Details: e.details}
	case *RangeError:
		return OperationResult{Error: rangeError, Details: e.details}
	case *TimedOut:
		return OperationResult{Error: timedOut, Details: e.details}
	case *NotSupported:
		return OperationResult{Error: notSupported, Details: e.details}
	case *Aborted:
		return OperationResult{Error: aborted, Details: e.details}
	case *NotOwner:
		return OperationResult{Error: notOwner, Details: e.details}
	default:
		return OperationResult{Error: e.Error()}
	}
}

// CheckOperationResults checks whether the provided operation was a success
// If the operation was a success, it will return nil, nil
// If the operation failed, due to a error committing the transaction it will
// return nil, error.
// Finally, in the case where one or more of the operations in the transaction
// failed, we return []OperationErrors, error
// Within []OperationErrors, the OperationErrors.Index() corresponds to the same index in
// the original Operations struct. You may also perform type assertions against
// the error so the caller can decide how best to handle it
func CheckOperationResults(result []OperationResult, ops []Operation) ([]OperationError, error) {
	// this shouldn't happen, but we'll cover the case to be certain
	if len(result) < len(ops) {
		return nil, fmt.Errorf("ovsdb transaction error. %d operations submitted but only %d results received", len(ops), len(result))
	}
	var errs []OperationError
	for i, op := range result {
		// RFC 7047: if all of the operations succeed, but the results cannot
		// be committed, then "result" will have one more element than "params",
		// with the additional element being an <error>.
		if i >= len(ops) {
			return errs, errorFromResult(nil, op)
		}
		if err := errorFromResult(&ops[i], op); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs, fmt.Errorf("%d ovsdb operations failed", len(errs))
	}
	return nil, nil
}

// OperationError represents an error that occurred as part of an
// OVSDB Operation
type OperationError interface {
	error
	// Operation is a pointer to the operation which caused the error
	Operation() *Operation
}

// ReferentialIntegrityViolation is explained in RFC 7047 4.1.3
type ReferentialIntegrityViolation struct {
	details   string
	operation *Operation
}

func NewReferentialIntegrityViolation(details string) *ReferentialIntegrityViolation {
	return &ReferentialIntegrityViolation{details: details}
}

// Error implements the error interface
func (e *ReferentialIntegrityViolation) Error() string {
	msg := referentialIntegrityViolation
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *ReferentialIntegrityViolation) Operation() *Operation {
	return e.operation
}

// ConstraintViolation is described in RFC 7047: 4.1.3
type ConstraintViolation struct {
	details   string
	operation *Operation
}

func NewConstraintViolation(details string) *ConstraintViolation {
	return &ConstraintViolation{details: details}
}

// Error implements the error interface
func (e *ConstraintViolation) Error() string {
	msg := constraintViolation
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *ConstraintViolation) Operation() *Operation {
	return e.operation
}

// ResourcesExhausted is described in RFC 7047: 4.1.3
type ResourcesExhausted struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *ResourcesExhausted) Error() string {
	msg := resourcesExhausted
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *ResourcesExhausted) Operation() *Operation {
	return e.operation
}

// IOError is described in RFC7047: 4.1.3
type IOError struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *IOError) Error() string {
	msg := ioError
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *IOError) Operation() *Operation {
	return e.operation
}

// DuplicateUUIDName is described in RFC7047 5.2.1
type DuplicateUUIDName struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *DuplicateUUIDName) Error() string {
	msg := duplicateUUIDName
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *DuplicateUUIDName) Operation() *Operation {
	return e.operation
}

// DomainError is described in RFC 7047: 5.2.4
type DomainError struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *DomainError) Error() string {
	msg := domainError
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *DomainError) Operation() *Operation {
	return e.operation
}

// RangeError is described in RFC 7047: 5.2.4
type RangeError struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *RangeError) Error() string {
	msg := rangeError
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *RangeError) Operation() *Operation {
	return e.operation
}

// TimedOut is described in RFC 7047: 5.2.6
type TimedOut struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *TimedOut) Error() string {
	msg := timedOut
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *TimedOut) Operation() *Operation {
	return e.operation
}

// NotSupported is described in RFC 7047: 5.2.7
type NotSupported struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *NotSupported) Error() string {
	msg := notSupported
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *NotSupported) Operation() *Operation {
	return e.operation
}

// Aborted is described in RFC 7047: 5.2.8
type Aborted struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *Aborted) Error() string {
	msg := aborted
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *Aborted) Operation() *Operation {
	return e.operation
}

// NotOwner is described in RFC 7047: 5.2.9
type NotOwner struct {
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *NotOwner) Error() string {
	msg := notOwner
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *NotOwner) Operation() *Operation {
	return e.operation
}

// Error is a generic OVSDB Error type that implements the
// OperationError and error interfaces
type Error struct {
	name      string
	details   string
	operation *Operation
}

// Error implements the error interface
func (e *Error) Error() string {
	msg := e.name
	if e.details != "" {
		msg += ": " + e.details
	}
	return msg
}

// Operation implements the OperationError interface
func (e *Error) Operation() *Operation {
	return e.operation
}

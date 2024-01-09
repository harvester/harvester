package types

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// Validator is a Mutator that doesn't modify received API objects.
type Validator interface {
	// Create checks if a CREATE operation is allowed. If no error is returned, the operation is allowed.
	Create(request *Request, newObj runtime.Object) error

	// Update checks if a UPDATE operation is allowed. If no error is returned, the operation is allowed.
	Update(request *Request, oldObj runtime.Object, newObj runtime.Object) error

	// Delete checks if a DELETE operation is allowed. If no error is returned, the operation is allowed.
	Delete(request *Request, oldObj runtime.Object) error

	// Connect checks if a CONNECT operation is allowed. If no error is returned, the operation is allowed.
	Connect(request *Request, newObj runtime.Object) error

	Resource() Resource
}

// ValidatorAdapter adapts a Validator to an Admitter.
type ValidatorAdapter struct {
	validator Validator
}

func NewValidatorAdapter(validator Validator) Mutator {
	return &ValidatorAdapter{validator: validator}
}

func (c *ValidatorAdapter) Create(request *Request, newObj runtime.Object) (PatchOps, error) {
	return nil, c.validator.Create(request, newObj)
}

func (c *ValidatorAdapter) Update(request *Request, oldObj runtime.Object, newObj runtime.Object) (PatchOps, error) {
	return nil, c.validator.Update(request, oldObj, newObj)
}

func (c *ValidatorAdapter) Delete(request *Request, oldObj runtime.Object) (PatchOps, error) {
	return nil, c.validator.Delete(request, oldObj)
}

func (c *ValidatorAdapter) Connect(request *Request, newObj runtime.Object) (PatchOps, error) {
	return nil, c.validator.Connect(request, newObj)
}

func (c *ValidatorAdapter) Resource() Resource {
	return c.validator.Resource()
}

// DefaultValidator allows every supported operation.
type DefaultValidator struct {
}

func (v *DefaultValidator) Create(_ *Request, _ runtime.Object) error {
	return nil
}

func (v *DefaultValidator) Update(_ *Request, _ runtime.Object, _ runtime.Object) error {
	return nil
}

func (v *DefaultValidator) Delete(_ *Request, _ runtime.Object) error {
	return nil
}

func (v *DefaultValidator) Connect(_ *Request, _ runtime.Object) error {
	return nil
}

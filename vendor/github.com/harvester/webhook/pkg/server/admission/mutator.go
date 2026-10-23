package admission

import (
	"k8s.io/apimachinery/pkg/runtime"
)

type Mutator Admitter

// DefaultMutator allows every supported operation and mutate nothing
type DefaultMutator struct{}

// DefaultMutator implements interface Mutator
var _ Mutator = &DefaultMutator{}

func (v *DefaultMutator) Create(request *Request, newObj runtime.Object) (Patch, error) {
	return nil, nil
}

func (v *DefaultMutator) Update(request *Request, oldObj runtime.Object, newObj runtime.Object) (Patch, error) {
	return nil, nil
}

func (v *DefaultMutator) Delete(request *Request, oldObj runtime.Object) (Patch, error) {
	return nil, nil
}

func (v *DefaultMutator) Connect(request *Request, newObj runtime.Object) (Patch, error) {
	return nil, nil
}

func (v *DefaultMutator) Resource() Resource {
	return Resource{}
}

package types

import (
	"k8s.io/apimachinery/pkg/runtime"
)

type Mutator Admitter

// DefaultMutator allows every supported operation and mutate nothing
type DefaultMutator struct {
}

func (v *DefaultMutator) Create(_ *Request, _ runtime.Object) (PatchOps, error) {
	return nil, nil
}

func (v *DefaultMutator) Update(_ *Request, _ runtime.Object, _ runtime.Object) (PatchOps, error) {
	return nil, nil
}

func (v *DefaultMutator) Delete(_ *Request, _ runtime.Object) (PatchOps, error) {
	return nil, nil
}

func (v *DefaultMutator) Connect(_ *Request, _ runtime.Object) (PatchOps, error) {
	return nil, nil
}

package types

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Resource struct {
	Name           string
	Scope          admissionregv1.ScopeType
	APIGroup       string
	APIVersion     string
	ObjectType     runtime.Object
	OperationTypes []admissionregv1.OperationType
}

func (r Resource) Validate() error {
	if r.Name == "" {
		return errUndefined("Name")
	}
	if r.Scope == "" {
		return errUndefined("Scope")
	}
	if r.APIGroup == "" {
		return errUndefined("APIGroup")
	}
	if r.APIVersion == "" {
		return errUndefined("APIVersion")
	}
	if r.ObjectType == nil {
		return errUndefined("ObjectType")
	}
	if r.OperationTypes == nil {
		return errUndefined("OperationTypes")
	}
	return nil
}

func errUndefined(field string) error {
	return fmt.Errorf("filed %s is not defined", field)
}

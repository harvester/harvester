package rancher

import (
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	admissionregv1 "k8s.io/api/admissionregistration/v1"

	"github.com/harvester/harvester/pkg/webhook/types"
)

// projectRoleTemplateBindings validation rules ensure no changes are allowed to the prtb objects other
// than by the list of predefined service account
type projectRoleTemplateBindings struct {
	rancherValidator
}

func NewProjectRoleTemplateBindingValidator() types.Validator {
	return &projectRoleTemplateBindings{}
}

func (r *projectRoleTemplateBindings) Resource() types.Resource {
	return types.Resource{
		Names:          []string{"projectroletemplatebindings"},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       "management.cattle.io",
		APIVersion:     "v3",
		ObjectType:     &managementv3.ProjectRoleTemplateBinding{},
		OperationTypes: []admissionregv1.OperationType{"*"},
	}
}

// project validation rules ensure no changes are allowed to the project objects other
// than by the list of predefined service account
type project struct {
	rancherValidator
}

func NewProjectValidator() types.Validator {
	return &project{}
}

func (r *project) Resource() types.Resource {
	return types.Resource{
		Names:          []string{"projects"},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       "management.cattle.io",
		APIVersion:     "v3",
		ObjectType:     &managementv3.ProjectRoleTemplateBinding{},
		OperationTypes: []admissionregv1.OperationType{"*"},
	}
}

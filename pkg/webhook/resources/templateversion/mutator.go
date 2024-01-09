package templateversion

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator() types.Mutator {
	return &templateVersionMutator{}
}

type templateVersionMutator struct {
	types.DefaultMutator
}

func newResource(ops []admissionregv1.OperationType) types.Resource {
	return types.Resource{
		Names:          []string{v1beta1.VirtualMachineTemplateVersionResourceName},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       v1beta1.SchemeGroupVersion.Group,
		APIVersion:     v1beta1.SchemeGroupVersion.Version,
		ObjectType:     &v1beta1.VirtualMachineTemplateVersion{},
		OperationTypes: ops,
	}
}

func (m *templateVersionMutator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
	})
}

func (m *templateVersionMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vmTemplVersion := newObj.(*v1beta1.VirtualMachineTemplateVersion)

	templateID := vmTemplVersion.Spec.TemplateID
	if templateID == "" {
		return nil, werror.NewInvalidError("TemplateId is empty", fieldTemplateID)
	}

	_, templateName := ref.Parse(templateID)

	// Do not generate a name if there is a name.
	if vmTemplVersion.Name != "" {
		return nil, nil
	}

	// patch "metadata.generateName" with "{templateName}-"
	var patchOps types.PatchOps
	patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/metadata/generateName", "value": "%s"}`, templateName+"-"))
	return patchOps, nil
}

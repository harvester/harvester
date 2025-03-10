package volumeimportsource

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	BlankSourceName = "filesystem-blank-source"
)

func NewValidator() types.Validator {
	return &dataVolumeValidator{}
}

type dataVolumeValidator struct {
	types.DefaultValidator
}

func (v *dataVolumeValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"volumeimportsources"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cdiv1.SchemeGroupVersion.Group,
		APIVersion: cdiv1.SchemeGroupVersion.Version,
		ObjectType: &cdiv1.VolumeImportSource{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}

func (v *dataVolumeValidator) Delete(request *types.Request, curObj runtime.Object) error {
	if request.IsGarbageCollection() || request.IsFromController() {
		return nil
	}

	curVolImportSource := curObj.(*cdiv1.VolumeImportSource)
	if curVolImportSource.Name == BlankSourceName {
		return werror.NewInvalidError(fmt.Sprintf("can not delete default blank source %s", BlankSourceName), "")
	}

	return nil
}

package version

import (
	"fmt"
	"strconv"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	MinFreeDiskSpaceGBAnnotation = "harvesterhci.io/minFreeDiskSpaceGB"
)

func NewValidator() types.Validator {
	return &versionValidator{}
}

type versionValidator struct {
	types.DefaultValidator
}

func (v *versionValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VersionResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Version{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *versionValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newVersion := newObj.(*v1beta1.Version)
	return v.checkAnnotations(newVersion)
}

func (v *versionValidator) checkAnnotations(version *v1beta1.Version) error {
	if value, ok := version.Annotations[MinFreeDiskSpaceGBAnnotation]; ok {
		_, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return werror.NewBadRequest(fmt.Sprintf("invalid value %s for annotation %s", value, MinFreeDiskSpaceGBAnnotation))
		}
	}
	return nil
}

func (v *versionValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	newVersion := newObj.(*v1beta1.Version)
	return v.checkAnnotations(newVersion)
}

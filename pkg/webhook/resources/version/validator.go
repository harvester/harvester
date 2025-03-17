package version

import (
	"fmt"
	"regexp"
	"strconv"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var (
	SHA512Pattern = regexp.MustCompile(`^[a-f0-9]{128}$`)
)

const (
	SkipGarbageCollectionThreadholdCheckAnnotation = "harvesterhci.io/skipGarbageCollectionThresholdCheck"
	MinCertsExpirationInDayAnnotation              = "harvesterhci.io/minCertsExpirationInDay"
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
	return checkVersion(newVersion)
}

func checkAnnotations(version *v1beta1.Version) error {
	if value, ok := version.Annotations[SkipGarbageCollectionThreadholdCheckAnnotation]; ok {
		_, err := strconv.ParseBool(value)
		if err != nil {
			return werror.NewBadRequest(fmt.Sprintf("invalid value %s for annotation %s", value, SkipGarbageCollectionThreadholdCheckAnnotation))
		}
	}

	if value, ok := version.Annotations[MinCertsExpirationInDayAnnotation]; ok {
		minCertsExpirationInDay, err := strconv.Atoi(value)
		if err != nil {
			return werror.NewBadRequest(fmt.Sprintf("invalid value %s for annotation %s", value, MinCertsExpirationInDayAnnotation))
		} else if minCertsExpirationInDay <= 0 {
			return werror.NewBadRequest(fmt.Sprintf("invalid value %s for annotation %s, it should be greater than 0", value, MinCertsExpirationInDayAnnotation))
		}
	}
	return nil
}

func (v *versionValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	newVersion := newObj.(*v1beta1.Version)
	return checkVersion(newVersion)
}

func checkVersion(version *v1beta1.Version) error {
	if err := checkAnnotations(version); err != nil {
		return err
	}
	return checkISOChecksum(version)
}

func checkISOChecksum(version *v1beta1.Version) error {
	isoChecksum := version.Spec.ISOChecksum
	// if an isoChecksum is provided, it must be in the SHA-512 format
	// since Longhorn backing images only accept hashes in that format
	if isoChecksum != "" && !SHA512Pattern.MatchString(isoChecksum) {
		return werror.NewBadRequest(fmt.Sprintf("invalid isoChecksum %s", isoChecksum))
	}
	return nil
}

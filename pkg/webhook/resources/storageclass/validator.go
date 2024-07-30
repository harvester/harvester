package storageclass

import (
	"fmt"
	"strings"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(storageClassCache ctlstoragev1.StorageClassCache) types.Validator {
	return &storageClassValidator{
		storageClassCache: storageClassCache,
	}
}

type storageClassValidator struct {
	types.DefaultValidator
	storageClassCache ctlstoragev1.StorageClassCache
}

func (v *storageClassValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"storageclasses"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   storagev1.SchemeGroupVersion.Group,
		APIVersion: storagev1.SchemeGroupVersion.Version,
		ObjectType: &storagev1.StorageClass{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *storageClassValidator) Create(_ *types.Request, newObj runtime.Object) error {
	err := v.validateSetUniqueDefault(newObj)
	if err != nil {
		return err
	}

	return v.validateDataLocality(newObj)
}

func (v *storageClassValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	return v.validateSetUniqueDefault(newObj)
}

// Harvester rejects setting dataLocality as strict-local, because it makes volume non migrateable,
// beside, strict-local volumes only could have one replica, it make Longhorn block node drain
// https://longhorn.io/docs/1.5.3/references/settings/#node-drain-policy
func (v *storageClassValidator) validateDataLocality(newObj runtime.Object) error {
	sc := newObj.(*storagev1.StorageClass)
	dataLocality, find := sc.Parameters[util.LonghornDataLocality]
	if !find {
		return nil
	}

	lhDataLocality := longhorn.DataLocality(dataLocality)

	if lhDataLocality == longhorn.DataLocalityStrictLocal {
		return werror.NewInvalidError("storage class with strict-local data locality is not allowed", "")
	}

	if lhDataLocality != longhorn.DataLocalityDisabled && lhDataLocality != longhorn.DataLocalityBestEffort {
		message := fmt.Sprintf("storage class with invalid data locality %v, valid values: %v",
			lhDataLocality, strings.Join([]string{`"disabled"`, `"best-effort"`}, ", "))
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *storageClassValidator) validateSetUniqueDefault(newObj runtime.Object) error {
	newSC := newObj.(*storagev1.StorageClass)

	// if new storage class haven't storageclass.kubernetes.io/is-default-class annotation or the value is false.
	if v, ok := newSC.Annotations[util.AnnotationIsDefaultStorageClassName]; !ok || v == "false" {
		return nil
	}

	scList, err := v.storageClassCache.List(labels.Everything())
	if err != nil {
		return werror.NewInvalidError(err.Error(), "")
	}

	for _, sc := range scList {
		if sc.Name == newSC.Name {
			continue
		}

		// return set default error
		// when find another have storageclass.kubernetes.io/is-default-class annotation and value is true.
		if v, ok := sc.Annotations[util.AnnotationIsDefaultStorageClassName]; ok && v == "true" {
			return werror.NewInvalidError("default storage class %s already exists, please reset it first", sc.Name)
		}
	}

	return nil
}

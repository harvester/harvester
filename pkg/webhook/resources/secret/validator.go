package secret

import (
	"fmt"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(storageClassCache ctlstoragev1.StorageClassCache) types.Validator {

	return &secretValidator{
		storageClassCache: storageClassCache,
	}
}

type secretValidator struct {
	types.DefaultValidator
	storageClassCache ctlstoragev1.StorageClassCache
}

func (v *secretValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"secrets"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Secret{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}

func (v *secretValidator) Delete(_ *types.Request, obj runtime.Object) error {
	secret := obj.(*corev1.Secret)

	return v.checkStorageClass(secret)
}

func (v *secretValidator) checkStorageClass(secret *corev1.Secret) error {
	storageClasses, err := v.storageClassCache.GetByIndex(indexeres.StorageClassBySecretIndex, fmt.Sprintf("%s/%s", secret.Namespace, secret.Name))
	if err != nil {
		return err
	}

	if len(storageClasses) == 0 {
		return nil
	}

	storageClassNames := make([]string, 0, len(storageClasses))
	for _, storageClass := range storageClasses {
		storageClassNames = append(storageClassNames, storageClass.Name)
	}

	return werror.NewInvalidError(fmt.Sprintf("Secret is used by encrypted storage classes: %s", storageClassNames), "")
}

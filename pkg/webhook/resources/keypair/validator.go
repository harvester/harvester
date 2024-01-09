package keypair

import (
	"errors"

	"golang.org/x/crypto/ssh"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldPublicKey = "spec.publicKey"
)

func NewValidator(keypairs ctlharvesterv1.KeyPairCache) types.Validator {
	return &keyPairValidator{
		keypairs: keypairs,
	}
}

type keyPairValidator struct {
	types.DefaultValidator
	keypairs ctlharvesterv1.KeyPairCache
}

func (v *keyPairValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.KeyPairResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.KeyPair{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *keyPairValidator) Create(_ *types.Request, newObj runtime.Object) error {
	keypair := newObj.(*v1beta1.KeyPair)

	if err := v.checkPublicKey(keypair.Spec.PublicKey); err != nil {
		return werror.NewInvalidError(err.Error(), fieldPublicKey)
	}
	return nil
}

func (v *keyPairValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	keypair := newObj.(*v1beta1.KeyPair)

	if err := v.checkPublicKey(keypair.Spec.PublicKey); err != nil {
		return werror.NewInvalidError(err.Error(), fieldPublicKey)
	}
	return nil
}

func (v *keyPairValidator) checkPublicKey(publicKey string) error {
	if publicKey == "" {
		return errors.New("public key is required")
	}

	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey)); err != nil {
		return errors.New("key is not in valid OpenSSH public key format")
	}

	return nil
}

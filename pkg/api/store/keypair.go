package store

import (
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"golang.org/x/crypto/ssh"
)

type KeyPairStore struct {
	types.Store
}

func (s KeyPairStore) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	if err := validatePublicKey(data); err != nil {
		return data, err
	}
	return s.Store.Create(apiOp, schema, data)
}

func validatePublicKey(data types.APIObject) error {
	publicKey := data.Data().String("spec", "publicKey")
	if publicKey == "" {
		return apierror.NewAPIError(validation.MissingRequired, "public key is required")
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey)); err != nil {
		return apierror.NewAPIError(validation.InvalidFormat, "key is not in valid OpenSSH public key format")
	}
	return nil
}

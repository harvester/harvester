package store

import (
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
)

type DisplayNameValidatorStore struct {
	types.Store
}

func (s DisplayNameValidatorStore) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	if err := s.validateDisplayName(apiOp, schema, data); err != nil {
		return data, err
	}
	return s.Store.Create(apiOp, schema, data)
}

func (s DisplayNameValidatorStore) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	if err := s.validateDisplayName(apiOp, schema, data); err != nil {
		return data, err
	}
	return s.Store.Update(apiOp, schema, data, id)
}
func (s DisplayNameValidatorStore) validateDisplayName(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) error {
	displayName := data.Data().String("spec", "displayName")
	if displayName == "" {
		return apierror.NewAPIError(validation.MissingRequired, "Name is required.")
	}
	list, err := s.List(apiOp, schema)
	if err != nil {
		return err
	}
	for _, obj := range list.Objects {
		if obj.Data().String("spec", "displayName") == displayName && obj.Name() != data.Name() {
			return apierror.NewAPIError(validation.Conflict, "A resource with the same name exists.")
		}
	}
	return nil
}

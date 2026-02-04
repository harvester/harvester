package userpreferences

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
)

type UserPreference struct {
	Data map[string]string `json:"data"`
}

func Register(schemas *types.APISchemas) {
	schemas.InternalSchemas.TypeName("userpreference", UserPreference{})
	schemas.MustImportAndCustomize(UserPreference{}, func(schema *types.APISchema) {
		schema.CollectionMethods = []string{http.MethodGet}
		schema.ResourceMethods = []string{http.MethodGet, http.MethodPut, http.MethodDelete}
		schema.Store = &localStore{}
	})
}

package subscribe

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
)

type SchemasGetter func(apiOp *types.APIRequest) *types.APISchemas

func DefaultGetter(apiOp *types.APIRequest) *types.APISchemas {
	return apiOp.Schemas
}

func Register(schemas *types.APISchemas, getter SchemasGetter, serverVersion string) {
	if getter == nil {
		getter = DefaultGetter
	}
	schemas.MustImportAndCustomize(Subscribe{}, func(schema *types.APISchema) {
		schema.CollectionMethods = []string{http.MethodGet}
		schema.ResourceMethods = []string{}
		schema.ListHandler = NewHandler(getter, serverVersion)
		schema.PluralName = "subscribe"
	})
}

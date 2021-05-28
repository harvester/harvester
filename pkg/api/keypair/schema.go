package keypair

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
)

const (
	keygen = "keygen"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(harvesterv1.KeyGenInput{}, nil)
	t := schema.Template{
		ID: "harvesterhci.io.keypair",
		Customize: func(s *types.APISchema) {
			s.CollectionFormatter = CollectionFormatter
			s.CollectionActions = map[string]schemas.Action{
				keygen: {
					Input: "keyGenInput",
				},
			}
			s.Formatter = Formatter
			s.ActionHandlers = map[string]http.Handler{
				keygen: KeyGenActionHandler{
					KeyPairs:     scaled.HarvesterFactory.Harvesterhci().V1beta1().KeyPair(),
					KeyPairCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache(),
				},
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}

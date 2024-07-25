package converter

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/util/proto"
)

// addDescription adds a description to all schemas in schemas using the openapi v2 definitions from k8s.
// Will not add new schemas, only mutate existing ones. Returns an error if the definitions could not be retrieved.
func addDescription(client discovery.DiscoveryInterface, schemas map[string]*types.APISchema) error {
	openapi, err := client.OpenAPISchema()
	if err != nil {
		return err
	}

	models, err := proto.NewOpenAPIData(openapi)
	if err != nil {
		return err
	}

	for _, modelName := range models.ListModels() {
		model := models.LookupModel(modelName)
		if k, ok := model.(*proto.Kind); ok {
			gvk := GetGVKForKind(k)
			if gvk == nil {
				// kind was not for top level gvk, we can skip this resource
				logrus.Tracef("when adding schema descriptions, will not add description for kind %s, which is not a top level resource", k.Path.String())
				continue
			}
			schemaID := GVKToVersionedSchemaID(*gvk)
			schema, ok := schemas[schemaID]
			// some kinds have a gvk but don't correspond to a schema (like a podList). We can
			// skip these resources as well
			if !ok {
				logrus.Tracef("when adding schema descriptions, will not add description for ID %s, which is not in schemas", schemaID)
				continue
			}
			schema.Description = k.GetDescription()
		}
	}

	return nil
}

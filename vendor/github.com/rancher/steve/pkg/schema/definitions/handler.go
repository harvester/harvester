package definitions

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema/converter"
	wapiextv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/util/proto"
)

var (
	internalServerErrorCode = validation.ErrorCode{
		Status: http.StatusInternalServerError,
		Code:   "InternalServerError",
	}
	notRefreshedErrorCode = validation.ErrorCode{
		Status: http.StatusServiceUnavailable,
		Code:   "SchemasNotRefreshed",
	}
)

type gvkModel struct {
	// ModelName is the name of the OpenAPI V2 model.
	// For example, the GVK Group=management.cattle.io/v2, Kind=UserAttribute will have
	// the following model name: io.cattle.management.v2.UserAttribute.
	ModelName string
	// Schema is the OpenAPI V2 schema for this GVK
	Schema proto.Schema
	// CRD is the schema from the CRD for this GVK, if there exists a CRD
	// for this group/kind
	CRD *apiextv1.JSONSchemaProps
}

// SchemaDefinitionHandler provides a schema definition for the schema ID provided. The schema definition is built
// using the following information, in order:
//
//  1. If the schema ID refers to a BaseSchema (a schema that doesn't exist in Kubernetes), then we use the
//     schema to return the schema definition - we return early. Otherwise:
//  2. We build a schema definition from the OpenAPI V2 info.
//  3. If the schemaID refers to a CRD, then we also build a schema definition from the CRD.
//  4. We merge both the OpenAPI V2 and the CRD schema definition. CRD will ALWAYS override whatever is
//     in OpenAPI V2. This makes sense because CRD is defined by OpenAPI V3, so has more information. This
//     merged schema definition is returned.
//
// Note: SchemaDefinitionHandler only implements a ByID handler. It does not implement any method allowing a caller
// to list definitions for all schemas.
type SchemaDefinitionHandler struct {
	// gvkModels maps a schema ID (eg: management.cattle.io.userattributes) to
	// the computed and cached gvkModel. It is recomputed on `Refresh()`.
	gvkModels map[string]gvkModel
	// models are the cached models from the last response from kubernetes.
	models proto.Models
	// lock protects gvkModels and models which are updated in Refresh
	lock sync.RWMutex

	// baseSchema are the schemas (which may not represent a real CRD) added to the server
	baseSchema *types.APISchemas

	// crdCache is used to add more information to a schema definition by getting information
	// from the CRD of the resource being accessed (if said resource is a CRD)
	crdCache wapiextv1.CustomResourceDefinitionCache

	// client is the discovery client used to get the groups/resources/fields from kubernetes
	client discovery.DiscoveryInterface
}

func NewSchemaDefinitionHandler(
	baseSchema *types.APISchemas,
	crdCache wapiextv1.CustomResourceDefinitionCache,
	client discovery.DiscoveryInterface,
) *SchemaDefinitionHandler {
	handler := &SchemaDefinitionHandler{
		baseSchema: baseSchema,
		crdCache:   crdCache,
		client:     client,
	}
	return handler
}

// Refresh writeLocks and updates the cache with new schemaDefinitions. Will result in a call to kubernetes to retrieve
// the openAPI schemas.
func (s *SchemaDefinitionHandler) Refresh() error {
	openapi, err := s.client.OpenAPISchema()
	if err != nil {
		return fmt.Errorf("unable to fetch openapi v2 definition: %w", err)
	}
	models, err := proto.NewOpenAPIData(openapi)
	if err != nil {
		return fmt.Errorf("unable to parse openapi definition into models: %w", err)
	}
	groups, err := s.client.ServerGroups()
	if err != nil {
		return fmt.Errorf("unable to retrieve groups: %w", err)
	}

	gvkModels, err := listGVKModels(models, groups, s.crdCache)
	if err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.gvkModels = gvkModels
	s.models = models
	return nil
}

// byIDHandler is the Handler method for a request to get the schema definition for a specific schema. Will use the
// cached models found during the last refresh as part of this process.
func (s *SchemaDefinitionHandler) byIDHandler(request *types.APIRequest) (types.APIObject, error) {
	// pseudo-access check, designed to make sure that users have access to the schema for the definition that they
	// are accessing.
	requestSchema := request.Schemas.LookupSchema(request.Name)
	if requestSchema == nil {
		return types.APIObject{}, apierror.NewAPIError(validation.NotFound, "no such schema")
	}

	if baseSchema := s.baseSchema.LookupSchema(requestSchema.ID); baseSchema != nil {
		// if this schema is a base schema it won't be in the model cache. In this case, and only this case, we process
		// the fields independently
		definitions := baseSchemaToDefinition(*requestSchema)
		return types.APIObject{
			ID:   request.Name,
			Type: "schemaDefinition",
			Object: schemaDefinition{
				DefinitionType: requestSchema.ID,
				Definitions:    definitions,
			},
		}, nil
	}

	s.lock.RLock()
	gvkModels := s.gvkModels
	protoModels := s.models
	s.lock.RUnlock()

	if gvkModels == nil || protoModels == nil {
		return types.APIObject{}, apierror.NewAPIError(notRefreshedErrorCode, "schema definitions not yet refreshed")
	}

	model, ok := gvkModels[requestSchema.ID]
	if !ok {
		return types.APIObject{}, apierror.NewAPIError(notRefreshedErrorCode, "no model found for schema, try again after refresh")
	}

	schemaDef, err := buildSchemaDefinitionForModel(protoModels, model)
	if err != nil {
		logrus.Errorf("failed building schema definition for model %s: %s", model.ModelName, err)
		return types.APIObject{}, apierror.NewAPIError(internalServerErrorCode, "failed building schema definition")
	}

	return types.APIObject{
		ID:     request.Name,
		Type:   "schemaDefinition",
		Object: schemaDef,
	}, nil
}

func buildSchemaDefinitionForModel(models proto.Models, gvk gvkModel) (schemaDefinition, error) {
	definitions, err := openAPIV2ToDefinition(gvk.Schema, models, gvk.ModelName)
	if err != nil {
		return schemaDefinition{}, fmt.Errorf("OpenAPI V2 to definition error: %w", err)
	}

	// CRDs don't always exists (eg: Pods, Deployments, etc)
	if gvk.CRD != nil {
		// CRD definitions generally has more information than the OpenAPI V2
		// because it embeds an OpenAPI V3 document. However, these 3 fields
		// are the exception where the Open API V2 endpoint has more
		// information.
		props := gvk.CRD.DeepCopy()
		delete(props.Properties, "apiVersion")
		delete(props.Properties, "kind")
		delete(props.Properties, "metadata")

		// We want to merge the OpenAPI V2 information with the CRD information
		// whenever possible because the CRD is defined by OpenAPI V3 which
		// _generally_ ends up with more information than OpenAPI V2
		// (eg: Optional fields wrongly ends up as type string in V2)
		crdDefinitions, err := crdToDefinition(props, gvk.ModelName)
		if err != nil {
			return schemaDefinition{}, fmt.Errorf("failed converting CRD to schema definition: %w", err)
		}

		if err := definitions.Merge(crdDefinitions); err != nil {
			return schemaDefinition{}, fmt.Errorf("merging V2 and CRD definition: %w", err)
		}
	}

	return definitions, nil
}

// listGVKModels returns a map of schemaID to the gvkModel. Will use the preferred version of a
// resource if possible.
func listGVKModels(models proto.Models, groups *metav1.APIGroupList, crdCache wapiextv1.CustomResourceDefinitionCache) (map[string]gvkModel, error) {
	groupToPreferredVersion := make(map[string]string)
	if groups != nil {
		for _, group := range groups.Groups {
			groupToPreferredVersion[group.Name] = group.PreferredVersion.Version
		}
	}

	gvkToCRD := make(map[schema.GroupVersionKind]*apiextv1.JSONSchemaProps)
	crds, err := crdCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, crd := range crds {
		for _, version := range crd.Spec.Versions {
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: version.Name,
				Kind:    crd.Spec.Names.Kind,
			}
			if version.Schema != nil {
				gvkToCRD[gvk] = version.Schema.OpenAPIV3Schema
			}
		}
	}

	schemaToGVKModel := map[string]gvkModel{}
	for _, modelName := range models.ListModels() {
		protoSchema := models.LookupModel(modelName)
		switch protoSchema.(type) {
		// It is possible that a Kubernetes resources ends up being treated as
		// a *proto.Map instead of *proto.Kind when it doesn't have any fields
		// defined. (eg: management.cattle.io.v1.UserAttributes)
		//
		// For that reason, we accept both *proto.Kind and *proto.Map
		// as long as they have a GVK assigned
		case *proto.Kind, *proto.Map:
		default:
			// no need to process models that aren't kind or map
			continue
		}

		// Makes sure the schema has a GVK (whether it's a Map or a Kind)
		gvk := converter.GetGVKForProtoSchema(protoSchema)
		if gvk == nil {
			continue
		}

		schemaID := converter.GVKToSchemaID(*gvk)

		prefVersion := groupToPreferredVersion[gvk.Group]
		_, ok := schemaToGVKModel[schemaID]
		// we always add the preferred version to the map. However, if this isn't the preferred version the preferred group could
		// be missing this resource (e.x. v1alpha1 has a resource, it's removed in v1). In those cases, we add the model name
		// only if we don't already have an entry. This way we always choose the preferred, if possible, but still have 1 version
		// for everything
		if !ok || prefVersion == gvk.Version {
			schemaToGVKModel[schemaID] = gvkModel{
				ModelName: modelName,
				Schema:    protoSchema,
				CRD:       gvkToCRD[*gvk],
			}
		}
	}
	return schemaToGVKModel, nil
}

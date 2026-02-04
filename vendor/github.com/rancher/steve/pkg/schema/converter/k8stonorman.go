// Package converter is responsible for converting the types registered with a k8s server to schemas which can be used
// by the UI (and other consumers) to discover the resources available and the current user's permissions.
package converter

import (
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/resources/apigroups"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/kube-openapi/pkg/util/proto"
)

const (
	gvkExtensionName    = "x-kubernetes-group-version-kind"
	gvkExtensionGroup   = "group"
	gvkExtensionVersion = "version"
	gvkExtensionKind    = "kind"
)

func GVKToVersionedSchemaID(gvk schema.GroupVersionKind) string {
	if gvk.Group == "" {
		return strings.ToLower(fmt.Sprintf("core.%s.%s", gvk.Version, gvk.Kind))
	}
	return strings.ToLower(fmt.Sprintf("%s.%s.%s", gvk.Group, gvk.Version, gvk.Kind))
}

func gvrToPluralName(gvr schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return fmt.Sprintf("core.%s.%s", gvr.Version, gvr.Resource)
	}
	return fmt.Sprintf("%s.%s.%s", gvr.Group, gvr.Version, gvr.Resource)
}

func GVKToSchemaID(gvk schema.GroupVersionKind) string {
	if gvk.Group == "" {
		return strings.ToLower(gvk.Kind)
	}
	return strings.ToLower(fmt.Sprintf("%s.%s", gvk.Group, gvk.Kind))
}

func GVRToPluralName(gvr schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return gvr.Resource
	}
	return fmt.Sprintf("%s.%s", gvr.Group, gvr.Resource)
}

// GetGVKForProto attempts to retrieve a GVK for a given OpenAPI V2 schema
// object.
// The GVK is defined in an extension. It is possible that the protoSchema does
// not have the GVK extension set - in that case, we return nil.
func GetGVKForProtoSchema(protoSchema proto.Schema) *schema.GroupVersionKind {
	extensions, ok := protoSchema.GetExtensions()[gvkExtensionName].([]any)
	if !ok {
		return nil
	}
	for _, extension := range extensions {
		if gvkExtension, ok := extension.(map[any]any); ok {
			gvk := schema.GroupVersionKind{
				Group:   convert.ToString(gvkExtension[gvkExtensionGroup]),
				Version: convert.ToString(gvkExtension[gvkExtensionVersion]),
				Kind:    convert.ToString(gvkExtension[gvkExtensionKind]),
			}
			return &gvk
		}
	}
	return nil
}

// GetGVKForKind attempts to retrieve a GVK for a given Kind. Not all kind represent top level resources,
// so this function may return nil if the kind did not have a gvk extension
func GetGVKForKind(kind *proto.Kind) *schema.GroupVersionKind {
	return GetGVKForProtoSchema(kind)
}

// ToSchemas creates the schemas for a K8s server, using client to discover groups/resources, and crd to potentially
// add additional information about new fields/resources. Mostly ties together addDiscovery and addCustomResources.
func ToSchemas(crd v1.CustomResourceDefinitionClient, client discovery.DiscoveryInterface) (map[string]*types.APISchema, error) {
	result := map[string]*types.APISchema{}

	addTemplateBased(result)

	if err := addDiscovery(client, result); err != nil {
		return nil, err
	}

	if err := addCustomResources(crd, result); err != nil {
		return nil, err
	}

	if err := addDescription(client, result); err != nil {
		return nil, err
	}

	return result, nil
}

// some schemas are not based on real resources but are filled-in by a template later on. This function adds the base
// schema so that these endpoints are still recognizable in the api
func addTemplateBased(schemas map[string]*types.APISchema) {
	apiGroupGVK := attributes.GVK(&apigroups.BaseSchema)
	schemas[GVKToVersionedSchemaID(apiGroupGVK)] = &apigroups.BaseSchema
}

package definitions

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/types"
	wranglerDefinition "github.com/rancher/wrangler/v3/pkg/schemas/definition"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/kube-openapi/pkg/util/proto"
)

// crdToDefinition builds a schemaDefinition for a CustomResourceDefinition
func crdToDefinition(jsonSchemaProps *apiextv1.JSONSchemaProps, modelName string) (schemaDefinition, error) {
	path := proto.NewPath(modelName)

	definitions, err := convertJSONSchemaPropsToDefinition(*jsonSchemaProps, path)
	if err != nil {
		return schemaDefinition{}, err
	}

	return schemaDefinition{
		DefinitionType: modelName,
		Definitions:    definitions,
	}, nil
}

// convertJSONSchemaPropsToDefinition recurses through the given schema props of
// type object and adds each definition found to the map of definitions
//
// This supports all OpenAPI V3 types: boolean, number, integer, string, object and array
// as defined here: https://swagger.io/specification/v3/
func convertJSONSchemaPropsToDefinition(props apiextv1.JSONSchemaProps, path proto.Path) (map[string]definition, error) {
	definitions := make(map[string]definition)
	_, err := convertJSONSchemaPropsObject(&props, path, definitions)
	if err != nil {
		return definitions, err
	}
	return definitions, nil
}

func convertJSONSchemaProps(props *apiextv1.JSONSchemaProps, path proto.Path, definitions map[string]definition) (definitionField, error) {
	if props.Type != "object" && props.Type != "array" {
		return convertJSONSchemaPropsPrimitive(props), nil
	}

	if props.Type == "array" {
		return convertJSONSchemaPropsArray(props, path, definitions)
	}

	if len(props.Properties) > 0 {
		return convertJSONSchemaPropsObject(props, path, definitions)
	}

	return convertJSONSchemaPropsMap(props, path, definitions)
}

func convertJSONSchemaPropsObject(props *apiextv1.JSONSchemaProps, path proto.Path, definitions map[string]definition) (definitionField, error) {
	field := definitionField{
		Description: props.Description,
		Type:        path.String(),
	}

	// CRDs don't support references yet, but we guard against recursive
	// lookups to be safe
	if _, ok := definitions[path.String()]; ok {
		return field, nil
	}

	def := definition{
		Type:           path.String(),
		Description:    props.Description,
		ResourceFields: map[string]definitionField{},
	}

	requiredSet := make(map[string]struct{})
	for _, name := range props.Required {
		requiredSet[name] = struct{}{}
	}

	for name, prop := range props.Properties {
		subField, err := convertJSONSchemaProps(&prop, path.FieldPath(name), definitions)
		if err != nil {
			return definitionField{}, err
		}

		_, required := requiredSet[name]
		subField.Required = required
		def.ResourceFields[name] = subField
	}

	definitions[path.String()] = def

	return field, nil
}

func convertJSONSchemaPropsPrimitive(props *apiextv1.JSONSchemaProps) definitionField {
	return definitionField{
		Description: props.Description,
		Type:        getPrimitiveType(props.Type),
	}
}

func convertJSONSchemaPropsArray(props *apiextv1.JSONSchemaProps, path proto.Path, definitions map[string]definition) (definitionField, error) {
	field := definitionField{
		Description: props.Description,
		Type:        "array",
	}
	item := getItemsSchema(props)
	if item == nil {
		return definitionField{}, fmt.Errorf("array %q must have at least one item", path.String())
	}

	subField, err := convertJSONSchemaProps(item, path, definitions)
	if err != nil {
		return definitionField{}, err
	}

	field.SubType = subField.Type

	return field, nil
}

func convertJSONSchemaPropsMap(props *apiextv1.JSONSchemaProps, path proto.Path, definitions map[string]definition) (definitionField, error) {
	field := definitionField{
		Description: props.Description,
		Type:        "map",
	}
	if props.AdditionalProperties != nil && props.AdditionalProperties.Schema != nil {
		subField, err := convertJSONSchemaProps(props.AdditionalProperties.Schema, path, definitions)
		if err != nil {
			return definitionField{}, err
		}
		field.SubType = subField.Type
	} else {
		// Create the object in the definitions (won't recurse because
		// by this point, we know props doesn't have any properties)
		subField, err := convertJSONSchemaPropsObject(props, path, definitions)
		if err != nil {
			return definitionField{}, err
		}
		field.SubType = subField.Type
	}
	return field, nil

}

// typ is a OpenAPI V2 or V3 type
func getPrimitiveType(typ string) string {
	switch typ {
	case "integer", "number":
		return "int"
	default:
		return typ
	}
}

func getItemsSchema(props *apiextv1.JSONSchemaProps) *apiextv1.JSONSchemaProps {
	if props.Items == nil {
		return nil
	}

	if props.Items.Schema != nil {
		return props.Items.Schema
	} else if len(props.Items.JSONSchemas) > 0 {
		// Copied from previous code in steve. Unclear if this path is
		// ever taken because it seems to be unused even in k8s
		// libraries and explicitly forbidden in CRDs
		return &props.Items.JSONSchemas[0]
	}
	return nil
}

// proto.Ref has unexported fields so we must implement our own proto.Reference
// type.
var _ proto.Reference = (*openAPIV2Reference)(nil)
var _ proto.Schema = (*openAPIV2Reference)(nil)

// openAPIV2Reference will be visited by proto.Schema.Accept() as a
// proto.Reference
type openAPIV2Reference struct {
	proto.BaseSchema
	reference string
	subSchema proto.Schema
}

func (r *openAPIV2Reference) Accept(v proto.SchemaVisitor) {
	v.VisitReference(r)
}

func (r *openAPIV2Reference) Reference() string {
	return r.reference
}

func (r *openAPIV2Reference) SubSchema() proto.Schema {
	return r.subSchema
}

func (r *openAPIV2Reference) GetName() string {
	return fmt.Sprintf("Reference to %q", r.reference)
}

// mapToKind converts a *proto.Map to a *proto.Kind by keeping the same
// description, etc but also adding the 3 minimum fields - apiVersion, kind and
// metadata.
// This function assumes that the protoMap given is a top-level object (eg: a CRD).
func mapToKind(protoMap *proto.Map, models proto.Models) (*proto.Kind, error) {
	apiVersion := &proto.Primitive{
		BaseSchema: proto.BaseSchema{
			Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
			Path:        protoMap.Path.FieldPath("apiVersion"),
		},
		Type: "string",
	}
	kind := &proto.Primitive{
		BaseSchema: proto.BaseSchema{
			Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
			Path:        protoMap.Path.FieldPath("kind"),
		},
		Type: "string",
	}
	objectMetaPath := "io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"
	objectMetaModel := models.LookupModel(objectMetaPath)
	if objectMetaModel == nil {
		return nil, fmt.Errorf("OpenAPI V2 model %q not found", objectMetaPath)
	}
	metadata := &openAPIV2Reference{
		BaseSchema: proto.BaseSchema{
			Description: "Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata",
			Path:        protoMap.Path.FieldPath("metadata"),
		},
		reference: objectMetaPath,
		subSchema: objectMetaModel,
	}
	return &proto.Kind{
		BaseSchema: protoMap.BaseSchema,
		Fields: map[string]proto.Schema{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}, nil
}

// openAPIV2ToDefinition builds a schemaDefinition for the given schemaID based on
// Resource information from OpenAPI v2 endpoint
func openAPIV2ToDefinition(protoSchema proto.Schema, models proto.Models, modelName string) (schemaDefinition, error) {
	switch m := protoSchema.(type) {
	case *proto.Map:
		// If the schema is a *proto.Map, it will not have any Fields associated with it
		// even though all Kubernetes resources have at least apiVersion, kind and metadata.
		//
		// We transform this Map to a Kind and inject these fields
		var err error
		protoSchema, err = mapToKind(m, models)
		if err != nil {
			return schemaDefinition{}, fmt.Errorf("convert map to kind: %w", err)
		}
	case *proto.Kind:
	default:
		return schemaDefinition{}, fmt.Errorf("model for %s was type %T, not a *proto.Kind nor *proto.Map", modelName, protoSchema)
	}
	definitions := map[string]definition{}
	visitor := schemaFieldVisitor{
		definitions: definitions,
	}
	protoSchema.Accept(&visitor)

	return schemaDefinition{
		DefinitionType: modelName,
		Definitions:    definitions,
	}, nil
}

// baseSchemaToDefinition converts a given schema to the definition map. This should only be used with baseSchemas, whose definitions
// are expected to be set by another application and may not be k8s resources.
func baseSchemaToDefinition(schema types.APISchema) map[string]definition {
	definitions := map[string]definition{}
	def := definition{
		Description:    schema.Description,
		Type:           schema.ID,
		ResourceFields: map[string]definitionField{},
	}
	for fieldName, field := range schema.ResourceFields {
		fieldType, subType := parseFieldType(field.Type)
		def.ResourceFields[fieldName] = definitionField{
			Type:        fieldType,
			SubType:     subType,
			Description: field.Description,
			Required:    field.Required,
		}
	}
	definitions[schema.ID] = def
	return definitions
}

// parseFieldType parses a schemas.Field's type to a type (first return) and subType (second return)
func parseFieldType(fieldType string) (string, string) {
	subType := wranglerDefinition.SubType(fieldType)
	if wranglerDefinition.IsMapType(fieldType) {
		return "map", subType
	}
	if wranglerDefinition.IsArrayType(fieldType) {
		return "array", subType
	}
	if wranglerDefinition.IsReferenceType(fieldType) {
		return "reference", subType
	}
	return fieldType, ""
}

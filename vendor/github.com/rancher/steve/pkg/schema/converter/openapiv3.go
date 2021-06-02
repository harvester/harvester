package converter

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func modelV3ToSchema(name string, k *v1.JSONSchemaProps, schemasMap map[string]*types.APISchema) *types.APISchema {
	s := types.APISchema{
		Schema: &schemas.Schema{
			ID:             name,
			ResourceFields: map[string]schemas.Field{},
			Attributes:     map[string]interface{}{},
			Description:    k.Description,
		},
	}

	for fieldName, schemaField := range k.Properties {
		s.ResourceFields[fieldName] = toResourceField(name+"."+fieldName, schemaField, schemasMap)
	}

	for _, fieldName := range k.Required {
		if f, ok := s.ResourceFields[fieldName]; ok {
			f.Required = true
			s.ResourceFields[fieldName] = f
		}
	}

	if existing, ok := schemasMap[s.ID]; ok && len(existing.Attributes) > 0 {
		s.Attributes = existing.Attributes
	}
	schemasMap[s.ID] = &s

	for k, v := range s.ResourceFields {
		if types.ReservedFields[k] {
			s.ResourceFields["_"+k] = v
			delete(s.ResourceFields, k)
		}
	}

	return &s
}

func toResourceField(name string, schema v1.JSONSchemaProps, schemasMap map[string]*types.APISchema) schemas.Field {
	f := schemas.Field{
		Description: schema.Description,
		Nullable:    true,
		Create:      true,
		Update:      true,
	}
	var itemSchema *v1.JSONSchemaProps
	if schema.Items != nil {
		if schema.Items.Schema != nil {
			itemSchema = schema.Items.Schema
		} else if len(schema.Items.JSONSchemas) > 0 {
			itemSchema = &schema.Items.JSONSchemas[0]
		}
	}

	switch schema.Type {
	case "array":
		if itemSchema == nil {
			f.Type = "array[json]"
		} else if itemSchema.Type == "object" {
			f.Type = "array[" + name + "]"
			modelV3ToSchema(name, itemSchema, schemasMap)
		} else {
			f.Type = "array[" + itemSchema.Type + "]"
		}
	case "object":
		if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Type == "object" {
			f.Type = "map[" + name + "]"
			modelV3ToSchema(name, schema.AdditionalProperties.Schema, schemasMap)
		} else if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
			f.Type = "map[" + schema.AdditionalProperties.Schema.Type + "]"
		} else {
			f.Type = name
			modelV3ToSchema(name, &schema, schemasMap)
		}
	case "number":
		f.Type = "int"
	default:
		f.Type = schema.Type
	}

	if f.Type == "" {
		f.Type = "json"
	}

	return f
}

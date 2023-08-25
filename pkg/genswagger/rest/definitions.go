package rest

import (
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

var ignoreFieldsObjectMeta = []string{
	"generateName",
	"selfLink",
	"uid",
	"resourceVersion",
	"generation",
	"creationTimestamp",
	"deletionTimestamp",
	"deletionGracePeriodSeconds",
	"labels",
	"annotations",
	"ownerReferences",
	"finalizers",
	"clusterName",
	"managedFields",
}

var defaultDefinitionsChain = []DefinitionsFunc{
	MetaRequired,
}

type DefinitionsChain []DefinitionsFunc

type DefinitionsFunc func(map[string]common.OpenAPIDefinition)

func SetDefinitions(definitions map[string]common.OpenAPIDefinition) map[string]common.OpenAPIDefinition {
	for _, f := range defaultDefinitionsChain {
		f(definitions)
	}
	return definitions
}

// MetaRequired sets name, kind, and apiVersion to be required
func MetaRequired(definitions map[string]common.OpenAPIDefinition) {
	objectMetaKey := "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"
	if objectMeta, ok := definitions[objectMetaKey]; ok {
		objectMeta.Schema.Required = append(objectMeta.Schema.Required, "name")
		for _, field := range ignoreFieldsObjectMeta {
			delete(objectMeta.Schema.SchemaProps.Properties, field)
		}
		definitions[objectMetaKey] = objectMeta
	}

	for k, v := range definitions {
		_, hasKind := v.Schema.SchemaProps.Properties["kind"]
		_, hasAPIVersion := v.Schema.SchemaProps.Properties["apiVersion"]
		if hasKind && hasAPIVersion {
			v.Schema.SchemaProps.Required = append(v.Schema.SchemaProps.Required, "kind", "apiVersion")
		}
		v.Schema = cleanSchemaDescription(v.Schema)
		definitions[k] = v
	}
}

func cleanSchemaDescription(schema spec.Schema) spec.Schema {
	schema.Description = ""
	if schema.Properties != nil {
		for k := range schema.Properties {
			schema.Properties[k] = cleanSchemaDescription(schema.Properties[k])
		}
	}
	if schema.Items != nil && schema.Items.Len() > 0 {
		for k := range schema.Items.Schemas {
			schema.Items.Schemas[k] = cleanSchemaDescription(schema.Items.Schemas[k])
		}
	}
	return schema
}

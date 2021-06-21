package rest

import (
	"k8s.io/kube-openapi/pkg/common"
)

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
		definitions[objectMetaKey] = objectMeta
	}

	for k, v := range definitions {
		_, hasKind := v.Schema.SchemaProps.Properties["kind"]
		_, hasAPIVersion := v.Schema.SchemaProps.Properties["apiVersion"]
		if hasKind && hasAPIVersion {
			v.Schema.SchemaProps.Required = append(v.Schema.SchemaProps.Required, "kind", "apiVersion")
			definitions[k] = v
		}
	}
}

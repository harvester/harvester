package rest

import (
	spec "github.com/go-openapi/spec"
	common "k8s.io/kube-openapi/pkg/common"
)

func AddV3OpenAPIDefinitions(definitions map[string]common.OpenAPIDefinition, ref common.ReferenceCallback) {
	definitions["github.com/rancher/rancher/pkg/apis/management.cattle.io/v3.BasicLogin"] = schemaManagementCattleIOV3BasicLoginResponse(ref)
}

func schemaManagementCattleIOV3BasicLoginResponse(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "BasicLogin is a the input for local auth login action",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"username": {
						SchemaProps: spec.SchemaProps{
							Description: "Username for the login.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"password": {
						SchemaProps: spec.SchemaProps{
							Description: "Password for the login.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
			},
		},
	}
}

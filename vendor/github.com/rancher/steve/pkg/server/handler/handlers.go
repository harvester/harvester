package handler

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/schema"
)

func k8sAPI(sf schema.Factory, apiOp *types.APIRequest) {
	vars := map[string]string{
		"name":      apiOp.Request.PathValue("name"),
		"type":      apiOp.Request.PathValue("type"),
		"nameorns":  apiOp.Request.PathValue("nameorns"),
		"namespace": apiOp.Request.PathValue("namespace"),
	}

	apiOp.Name = vars["name"]
	apiOp.Type = vars["type"]

	nOrN := vars["nameorns"]
	if nOrN != "" {
		schema := apiOp.Schemas.LookupSchema(apiOp.Type)
		if attributes.Namespaced(schema) {
			apiOp.Namespace = nOrN
		} else {
			apiOp.Name = nOrN
		}
	}

	if namespace := vars["namespace"]; namespace != "" {
		apiOp.Namespace = namespace
	}
}

func apiRoot(sf schema.Factory, apiOp *types.APIRequest) {
	apiOp.Type = "apiRoot"
}

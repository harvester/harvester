package user

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	if request.Method == http.MethodGet {
		delete(resource.APIObject.Data(), "password")
	}
}

package user

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	if request.Method == http.MethodGet {
		delete(resource.APIObject.Data(), "password")
	}
}

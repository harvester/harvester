package vmtemplate

import (
	"github.com/rancher/apiserver/pkg/types"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Links["versions"] = request.URLBuilder.Link(resource.Schema, resource.ID, "versions")
}

func versionFormatter(_ *types.APIRequest, resource *types.RawResource) {
	delete(resource.Links, "update")
}

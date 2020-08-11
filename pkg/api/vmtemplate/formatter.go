package vmtemplate

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	resource.Links["versions"] = request.URLBuilder.Link(resource.Schema, resource.ID, "versions")
}

func versionFormatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	delete(resource.Links, "update")
}

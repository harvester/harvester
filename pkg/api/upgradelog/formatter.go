package upgradelog

import (
	"github.com/rancher/apiserver/pkg/types"
)

const (
	downloadArchiveLink   = "download"
	generateArchiveAction = "generate"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	resource.AddAction(request, generateArchiveAction)
}

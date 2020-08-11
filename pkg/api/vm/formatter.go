package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
)

const (
	startVM   = "startVM"
	stopVM    = "stopVM"
	restartVM = "restartVM"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)
	if resource.APIObject.Data().Bool("spec", "running") {
		resource.AddAction(request, stopVM)
		resource.AddAction(request, restartVM)
	} else {
		resource.AddAction(request, startVM)
	}
}

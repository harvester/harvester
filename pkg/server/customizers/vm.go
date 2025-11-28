package customizers

import (
	"strings"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/sirupsen/logrus"
)

/*
	 wrangler summarizers generate a generating warning as follows and we need to hide it if needed
		{
		  "error": false,
		  "message": "VirtualMachine generation is 5, but latest observed generation is 4",
		  "name": "in-progress",
		  "transitioning": true
		}
*/

func DropRevisionStateIfNeeded(request *types.APIRequest, resource *types.RawResource) {
	data := resource.APIObject.Data()
	state := data.Map("metadata", "state")
	logrus.Infof("found state %v\n", state)
	message := state["message"].(string)
	if strings.Contains(message, "but latest observed generation is") {
		state["error"] = false
		state["transitioning"] = false
		state["message"] = ""
		state["name"] = "running"
		data.SetNested(state, "metadata", "state")
	}
}

var VMCustomizerTemplate = schema.Template{
	ID:        "kubevirt.io.virtualmachine",
	Formatter: DropRevisionStateIfNeeded,
}

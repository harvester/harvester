package customizers

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var VMCustomizerTemplate = schema.Template{
	Group: "kubevirt.io",
	Kind:  "virtualmachine",
	Customize: func(schema *types.APISchema) {
		schema.CollectionFormatter = func(apiOp *types.APIRequest, collection *types.GenericCollection) {
			for _, d := range collection.Data {
				obj := d.APIObject.Object.(*unstructured.Unstructured)
				checkAndRemoveGenerationWarning(obj)
			}
		}
	},
}

/*
	 wrangeler summarizers generate a generating warning as follows and we need to hide it if needed
		{
		  "error": false,
		  "message": "VirtualMachine generation is 5, but latest observed generation is 4",
		  "name": "in-progress",
		  "transitioning": true
		}
*/
func checkAndRemoveGenerationWarning(obj *unstructured.Unstructured) {
	state, found, err := unstructured.NestedMap(obj.Object, "metadata", "state")
	if err != nil || !found {
		return
	}
	// attempt to tidy up status

	observedGeneration, found, err := unstructured.NestedInt64(obj.Object, "state", "observedGeneration")
	if err != nil || !found {
		return
	}
	generation := obj.GetGeneration()
	if generation != observedGeneration {
		message, ok := state["message"]
		if !ok {
			return
		}

		if message == fmt.Sprintf("VirtualMachine generation is %d, but latest observed generation is %d", generation, observedGeneration) {
			state["error"] = false
			state["message"] = ""
			state["name"] = "running"
			state["transitioning"] = false
			_ = unstructured.SetNestedMap(obj.Object, state, "metadata")
		}
	}

}

package formatters

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var namespaceGR = schema.GroupResource{Group: "", Resource: "namespaces"}

var allowedAnnotations = []string{
	"management.cattle.io/system-namespace", // system namespace detection
	"field.cattle.io/projectId",             // project grouping
}

var allowedLabels = []string{
	"fleet.cattle.io/managed", // fleet namespace detection
}

// Namespace strips sensitive metadata from namespace objects the user cannot actually "get".
// Preserves: name, resourceVersion, status.phase, and allowed annotations/labels for UI.
func Namespace(request *types.APIRequest, resource *types.RawResource) {
	accessSet := accesscontrol.AccessSetFromAPIRequest(request)
	if accessSet == nil {
		return
	}

	name := resource.APIObject.Name()
	if accessSet.Grants("get", namespaceGR, "", name) {
		return
	}

	unstr, ok := resource.APIObject.Object.(*unstructured.Unstructured)
	if !ok {
		return
	}

	// Build object with empty structures to avoid nil access issues in UI
	newObj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name":        name,
			"labels":      map[string]interface{}{},
			"annotations": map[string]interface{}{},
		},
		"spec":   map[string]interface{}{},
		"status": map[string]interface{}{},
	}
	metadata := newObj["metadata"].(map[string]interface{})

	if rv, ok, _ := unstructured.NestedString(unstr.Object, "metadata", "resourceVersion"); ok {
		metadata["resourceVersion"] = rv
	}

	if annotations, ok, _ := unstructured.NestedStringMap(unstr.Object, "metadata", "annotations"); ok {
		for _, key := range allowedAnnotations {
			if v, exists := annotations[key]; exists {
				metadata["annotations"].(map[string]interface{})[key] = v
			}
		}
	}

	if labels, ok, _ := unstructured.NestedStringMap(unstr.Object, "metadata", "labels"); ok {
		for _, key := range allowedLabels {
			if v, exists := labels[key]; exists {
				metadata["labels"].(map[string]interface{})[key] = v
			}
		}
	}

	if phase, ok, _ := unstructured.NestedString(unstr.Object, "status", "phase"); ok {
		newObj["status"].(map[string]interface{})["phase"] = phase
	}

	unstr.Object = newObj
}

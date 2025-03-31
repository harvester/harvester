package common

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

// GetUnstructured retrieves an unstructured object from the provided input. If this is a signal
// object (like cache.DeletedFinalStateUnknown), returns true, indicating that this wasn't an
// unstructured object, but doesn't need to be processed by our transform function
func GetUnstructured(obj any) (*unstructured.Unstructured, bool, error) {
	raw, ok := obj.(*unstructured.Unstructured)
	if !ok {
		_, isFinalUnknown := obj.(cache.DeletedFinalStateUnknown)
		if isFinalUnknown {
			// As documented in the TransformFunc interface
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("object was of type %T, not unstructured", raw)
	}
	return raw, false, nil
}

// toMap converts an object to a map[string]any which can be stored/retrieved from the cache. Currently
// uses json encoding to take advantage of tag names
func toMap(obj any) (map[string]any, error) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal object: %w", err)
	}
	var retObj map[string]any
	err = json.Unmarshal(bytes, &retObj)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal object: %w", err)
	}
	return retObj, nil
}

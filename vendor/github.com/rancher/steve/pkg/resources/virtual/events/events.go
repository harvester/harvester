// Package common provides cache.TransformFunc's for /v1 Event objects
package events

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TransformEventObject does special-case handling on event objects
// 1. (only one so far): replaces the _type field with the contents of the field named "type", if it exists
func TransformEventObject(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	currentTypeValue, ok := obj.Object["type"]
	if ok {
		obj.Object["_type"] = currentTypeValue
	}
	return obj, nil
}

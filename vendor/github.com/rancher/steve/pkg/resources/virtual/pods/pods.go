package pods

import (
	"github.com/rancher/norman/types/convert"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TransformPodObject does the following:
// 1. Ensure metadata.state.name is capitalized (and it should exist)
// 2. if metadata.fields[2] exists, set it to lowercase(metadata.state.name) -- might not exist
// data.SetNested(convert.LowerTitle(fields[2]), "metadata", "state", "name")
func TransformPodObject(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	res, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "fields")
	if !found || err != nil {
		// These probably don't exist yet
		return obj, nil
	}
	rawFields := res.([]interface{})
	if len(rawFields) <= 2 {
		return obj, nil
	}
	currentStateName, found, err := unstructured.NestedString(obj.Object, "metadata", "state", "name")
	if !found || err != nil {
		logrus.Errorf("Failed to get metadata.state.name from pod object %s: %v", obj.GetName(), err)
		return obj, nil
	}
	// convert.LowerTitle converts "Batman" to "batman" but "BatMobile" to "batMobile"
	mdfCamelCaseStateName := convert.LowerTitle(rawFields[2].(string))
	if mdfCamelCaseStateName != currentStateName {
		unstructured.SetNestedField(obj.Object, mdfCamelCaseStateName, "metadata", "state", "name")
	}
	return obj, nil
}

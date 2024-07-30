// Package yaml handles the unmarshaling of YAML objects to strusts
package yaml

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
	"github.com/rancher/wrangler/v3/pkg/gvk"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"
)

var (
	cleanPrefix = []string{
		"kubectl.kubernetes.io/",
	}
	cleanContains = []string{
		"cattle.io/",
	}
)

const buffSize = 4096

// Unmarshal decodes YAML bytes into document (as defined by
// the YAML spec) then converting it to JSON via
// "k8s.io/apimachinery/pkg/util/yaml".YAMLToJSON and then decoding the json in the the v interface{}
func Unmarshal(data []byte, v interface{}) error {
	return yamlDecoder.NewYAMLToJSONDecoder(bytes.NewBuffer(data)).Decode(v)
}

// UnmarshalWithJSONDecoder expects a reader of raw YAML.
// It converts the document or documents to JSON,
// then decodes the JSON bytes into a slice of values of type T.
// Type T must be a pointer, or the function will panic.
func UnmarshalWithJSONDecoder[T any](yamlReader io.Reader) ([]T, error) {
	// verify the object is a pointer
	var obj T
	objPtrType := reflect.TypeOf(obj)
	if objPtrType.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("Object T must be a pointer not %v", objPtrType))
	}
	objType := objPtrType.Elem()
	var result []T

	// create a reader that reads one YAML document at a time.
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(yamlReader, buffSize))
	for {
		// get raw yaml for a single document
		rawYAML, err := reader.Read()
		if errors.Is(err, io.EOF) {
			// finished reading
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read yaml: %w", err)
		}
		// convert YAML to JSON
		rawJSON, err := yamlDecoder.ToJSON(rawYAML)
		if err != nil {
			return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
		}
		// unmarshal JSON to the provided object
		newObj := reflect.New(objType).Interface().(T)
		err = json.Unmarshal(rawJSON, newObj)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal converted JSON: %w", err)
		}
		result = append(result, newObj)
	}
	return result, nil
}

// ToObjects takes a reader of yaml bytes and returns a list of unstructured.Unstructured runtime.Objects that are read.
// If one of the objects read is an unstructured.UnstructuredList then the list is flattened to individual objects.
func ToObjects(in io.Reader) ([]runtime.Object, error) {
	var result []runtime.Object
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(in, buffSize))
	for {
		raw, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		obj, err := toObjects(raw)
		if err != nil {
			return nil, err
		}

		result = append(result, obj...)
	}

	return result, nil
}

func toObjects(bytes []byte) ([]runtime.Object, error) {
	bytes, err := yamlDecoder.ToJSON(bytes)
	if err != nil {
		return nil, err
	}

	check := map[string]interface{}{}
	if err := json.Unmarshal(bytes, &check); err != nil || len(check) == 0 {
		return nil, err
	}

	obj, _, err := unstructured.UnstructuredJSONScheme.Decode(bytes, nil, nil)
	if err != nil {
		return nil, err
	}

	if l, ok := obj.(*unstructured.UnstructuredList); ok {
		var result []runtime.Object
		for _, obj := range l.Items {
			copy := obj
			result = append(result, &copy)
		}
		return result, nil
	}

	return []runtime.Object{obj}, nil
}

// Export will attempt to clean up the objects a bit before
// rendering to yaml so that they can easily be imported into another
// cluster
func Export(objects ...runtime.Object) ([]byte, error) {
	if len(objects) == 0 {
		return nil, nil
	}

	buffer := &bytes.Buffer{}
	for i, obj := range objects {
		if i > 0 {
			buffer.WriteString("\n---\n")
		}

		obj, err := CleanObjectForExport(obj)
		if err != nil {
			return nil, err
		}

		bytes, err := yaml.Marshal(obj)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encode %s", obj.GetObjectKind().GroupVersionKind())
		}
		buffer.Write(bytes)
	}

	return buffer.Bytes(), nil
}

func CleanObjectForExport(obj runtime.Object) (runtime.Object, error) {
	obj = obj.DeepCopyObject()
	if obj.GetObjectKind().GroupVersionKind().Kind == "" {
		if gvk, err := gvk.Get(obj); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		} else if err != nil {
			return nil, errors.Wrapf(err, "kind and/or apiVersion is not set on input object: %v", obj)
		}
	}

	data, err := convert.EncodeToMap(obj)
	if err != nil {
		return nil, err
	}

	unstr := &unstructured.Unstructured{
		Object: data,
	}

	metadata := map[string]interface{}{}

	if name := unstr.GetName(); len(name) > 0 {
		metadata["name"] = name
	} else if generated := unstr.GetGenerateName(); len(generated) > 0 {
		metadata["generateName"] = generated
	} else {
		return nil, fmt.Errorf("either name or generateName must be set on obj: %v", obj)
	}

	if unstr.GetNamespace() != "" {
		metadata["namespace"] = unstr.GetNamespace()
	}
	if annotations := unstr.GetAnnotations(); len(annotations) > 0 {
		cleanMap(annotations)
		if len(annotations) > 0 {
			metadata["annotations"] = annotations
		} else {
			delete(metadata, "annotations")
		}
	}
	if labels := unstr.GetLabels(); len(labels) > 0 {
		cleanMap(labels)
		if len(labels) > 0 {
			metadata["labels"] = labels
		} else {
			delete(metadata, "labels")
		}
	}

	if spec, ok := data["spec"]; ok {
		if spec == nil {
			delete(data, "spec")
		} else if m, ok := spec.(map[string]interface{}); ok && len(m) == 0 {
			delete(data, "spec")
		}
	}

	data["metadata"] = metadata
	delete(data, "status")

	return unstr, nil
}

func CleanAnnotationsForExport(annotations map[string]string) map[string]string {
	result := make(map[string]string, len(annotations))

outer:
	for k := range annotations {
		for _, prefix := range cleanPrefix {
			if strings.HasPrefix(k, prefix) {
				continue outer
			}
		}
		for _, contains := range cleanContains {
			if strings.Contains(k, contains) {
				continue outer
			}
		}
		result[k] = annotations[k]
	}
	return result
}

func cleanMap(annoLabels map[string]string) {
	for k := range annoLabels {
		for _, prefix := range cleanPrefix {
			if strings.HasPrefix(k, prefix) {
				delete(annoLabels, k)
			}
		}
	}
}

func ToBytes(objects []runtime.Object) ([]byte, error) {
	if len(objects) == 0 {
		return nil, nil
	}

	buffer := &bytes.Buffer{}
	for i, obj := range objects {
		if i > 0 {
			buffer.WriteString("\n---\n")
		}

		bytes, err := yaml.Marshal(obj)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encode %s", obj.GetObjectKind().GroupVersionKind())
		}
		buffer.Write(bytes)
	}

	return buffer.Bytes(), nil
}

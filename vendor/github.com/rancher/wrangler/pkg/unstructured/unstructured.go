package unstructured

import (
	"github.com/rancher/wrangler/pkg/data/convert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	if ustr, ok := obj.(*unstructured.Unstructured); ok {
		return ustr, nil
	}

	data, err := convert.EncodeToMap(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: data,
	}, nil
}

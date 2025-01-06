package kubernetes

import (
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetObjMetaAccesser(obj interface{}) (metav1.Object, error) {
	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		return nil, errors.Errorf("Failed to convert obj (%v) to runtime.Object", obj)
	}

	return meta.Accessor(runtimeObj)
}

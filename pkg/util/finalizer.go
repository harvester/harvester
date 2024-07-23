package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ContainsFinalizer(obj metav1.Object, finalizer string) bool {
	finalizers := obj.GetFinalizers()
	for _, v := range finalizers {
		if v == finalizer {
			return true
		}
	}

	return false
}

func AddFinalizer(obj metav1.Object, finalizer string) {
	if !ContainsFinalizer(obj, finalizer) {
		finalizers := obj.GetFinalizers()
		finalizers = append(finalizers, finalizer)
		obj.SetFinalizers(finalizers)
	}
}

func RemoveFinalizer(obj metav1.Object, finalizer string) {
	if ContainsFinalizer(obj, finalizer) {
		finalizers := obj.GetFinalizers()
		var newFinalizers []string
		for _, v := range finalizers {
			if v != finalizer {
				newFinalizers = append(newFinalizers, v)
			}
		}
		obj.SetFinalizers(newFinalizers)
	}
}

func GetWranglerFinalizerName(controllerName string) string {
	return "wrangler.cattle.io/" + controllerName
}

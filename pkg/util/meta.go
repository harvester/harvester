package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NamespacedName(namespace, name string) string {
	return types.NamespacedName{Namespace: namespace, Name: name}.String()
}

func GetNamespacedName(obj metav1.Object) string {
	return NamespacedName(obj.GetNamespace(), obj.GetName())
}

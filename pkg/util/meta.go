package util

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NamespacedName(namespace, name string) string {
	return types.NamespacedName{Namespace: namespace, Name: name}.String()
}

func GetNamespacedName(obj metav1.Object) string {
	return NamespacedName(obj.GetNamespace(), obj.GetName())
}

// SplitNamespacedName splits a string in the format "namespace/name" into its two components.
// If the input string does not contain a slash, it is assumed to be a non-namespaced name,
// and the function will return an empty namespace and the original string as the name.
func SplitNamespacedName(namespacedName string) (string, string, bool) {
	if namespacedName == "" || strings.HasPrefix(namespacedName, string(types.Separator)) || strings.HasSuffix(namespacedName, string(types.Separator)) {
		return "", "", false
	}
	parts := strings.Split(namespacedName, string(types.Separator))
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	if len(parts) == 1 {
		return "", parts[0], true
	}
	return "", "", false
}

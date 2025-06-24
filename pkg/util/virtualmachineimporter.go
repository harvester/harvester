package util

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// IsImportedByVMIC checks if the given object has been created by the vm-import-controller.
func IsImportedByVMIC(obj metav1.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	value, exists := labels[LabelVMimported]
	if !exists {
		return false
	}
	return value == "true"
}

package util

import (
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Get OnwerReference for an object
// This function extracts all necessary TypeMeta and ObjectMeta information from
// the struct of a Kubernetes API object and constructs an OwnerReference struct
// with that data.
func GetOwnerReferenceFor(obj any) (*metav1.OwnerReference, error) {
	t, err := apimeta.TypeAccessor(obj)
	if err != nil {
		return nil, err
	}
	m, err := apimeta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return &metav1.OwnerReference{
		APIVersion: t.GetAPIVersion(),
		Kind:       t.GetKind(),
		Name:       m.GetName(),
		UID:        m.GetUID(),
	}, nil
}

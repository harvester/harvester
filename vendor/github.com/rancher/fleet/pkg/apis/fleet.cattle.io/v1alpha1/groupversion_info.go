// Copyright (c) 2021-2023 SUSE LLC

// Package v1alpha1 contains API Schema definitions for the fleet.cattle.io v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=fleet.cattle.io
package v1alpha1

import (
	scheme "github.com/rancher/fleet/pkg/apis/internal"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: "fleet.cattle.io", Version: "v1alpha1"}

	// InternalSchemeBuilder is used to add go types to the GroupVersionKind scheme
	InternalSchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// Compatibility with k8s.io/apimachinery/pkg/runtime.Object
	SchemeBuilder = InternalSchemeBuilder.SchemeBuilder

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = InternalSchemeBuilder.AddToScheme
)

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

package v1alpha1

import (
	"bytes"
	"encoding/json"
)

// Resource contains metadata about the resources of a bundle.
type Resource struct {
	// APIVersion is the API version of the resource.
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind is the k8s kind of the resource.
	// +nullable
	Kind string `json:"kind,omitempty"`
	// Type is the type of the resource, e.g. "apiextensions.k8s.io.customresourcedefinition" or "configmap".
	Type string `json:"type,omitempty"`
	// ID is the name of the resource, e.g. "namespace1/my-config" or "backingimagemanagers.storage.io".
	// +nullable
	ID string `json:"id,omitempty"`
	// Namespace of the resource.
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// Name of the resource.
	// +nullable
	Name string `json:"name,omitempty"`
	// IncompleteState is true if a bundle summary has 10 or more non-ready
	// resources or a non-ready resource has more 10 or more non-ready or
	// modified states.
	IncompleteState bool `json:"incompleteState,omitempty"`
	// State is the state of the resource, e.g. "Unknown", "WaitApplied", "ErrApplied" or "Ready".
	State string `json:"state,omitempty"`
	// Error is true if any Error in the PerClusterState is true.
	Error bool `json:"error,omitempty"`
	// Transitioning is true if any Transitioning in the PerClusterState is true.
	Transitioning bool `json:"transitioning,omitempty"`
	// Message is the first message from the PerClusterStates.
	// +nullable
	Message string `json:"message,omitempty"`
	// PerClusterState contains lists of cluster IDs for every State for this resource
	// +nullable
	PerClusterState PerClusterState `json:"perClusterState"`
}

// PerClusterState aggregates list of cluster IDs per state for a given Resource
type PerClusterState struct {
	// Ready is a list of cluster IDs for which this a resource is in Ready state
	Ready []string `json:"ready,omitempty"`
	// WaitApplied is a list of cluster IDs for which this a resource is in WaitApplied state
	WaitApplied []string `json:"waitApplied,omitempty"`
	// Pending is a list of cluster IDs for which this a resource is in Pending state
	Pending []string `json:"pending,omitempty"`
	// Modified is a list of cluster IDs for which this a resource is in Modified state
	Modified []string `json:"modified,omitempty"`
	// Orphaned is a list of cluster IDs for which this a resource is in Orphaned state
	Orphaned []string `json:"orphaned,omitempty"`
	// Missing is a list of cluster IDs for which this a resource is in Missing state
	Missing []string `json:"missing,omitempty"`
	// Unknown is a list of cluster IDs for which this a resource is in Unknown state
	Unknown []string `json:"unknown,omitempty"`
	// NotReady is a list of cluster IDs for which this a resource is in NotReady state
	NotReady []string `json:"notReady,omitempty"`
}

type perClusterStateAlias PerClusterState

// UnmarshalJSON implements the json.Unmarshaler interface, to which the yaml decoders delegate to
// PerClusterState used to be a slice instead of a struct, which breaks backwards compatibility with existing data during upgrade
// Simply ignoring non-object inputs solves the problem, since the following reconciliations will update the resources to the correct payload
// To be removed for Fleet >= 0.13
func (in *PerClusterState) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(data, []byte("{")) {
		return nil
	}
	var aux perClusterStateAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*in = PerClusterState(aux)
	return nil
}

// ResourceCounts contains the number of resources in each state.
type ResourceCounts struct {
	// Ready is the number of ready resources.
	// +optional
	Ready int `json:"ready"`
	// DesiredReady is the number of resources that should be ready.
	// +optional
	DesiredReady int `json:"desiredReady"`
	// WaitApplied is the number of resources that are waiting to be applied.
	// +optional
	WaitApplied int `json:"waitApplied"`
	// Modified is the number of resources that have been modified.
	// +optional
	Modified int `json:"modified"`
	// Orphaned is the number of orphaned resources.
	// +optional
	Orphaned int `json:"orphaned"`
	// Missing is the number of missing resources.
	// +optional
	Missing int `json:"missing"`
	// Unknown is the number of resources in an unknown state.
	// +optional
	Unknown int `json:"unknown"`
	// NotReady is the number of not ready resources. Resources are not
	// ready if they do not match any other state.
	// +optional
	NotReady int `json:"notReady"`
}

package v1alpha1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeSlicePoolSpec defines the desired state of NodeSlicePool
type NodeSlicePoolSpec struct {
	// Range is a RFC 4632/4291-style string that represents an IP address and prefix length in CIDR notation
	// this refers to the entire range where the node is allocated a subset
	Range string `json:"range"`

	// SliceSize is the size of subnets or slices of the range that each node will be assigned
	SliceSize string `json:"sliceSize"`
}

// NodeSlicePoolStatus defines the desired state of NodeSlicePool
type NodeSlicePoolStatus struct {
	// Allocations holds the allocations of nodes to slices
	Allocations []NodeSliceAllocation `json:"allocations"`
}

type NodeSliceAllocation struct {
	// NodeName is the name of the node assigned to this slice, empty node name is an available slice for assignment
	NodeName string `json:"nodeName"`

	// SliceRange is the subnet of this slice
	SliceRange string `json:"sliceRange"`
}

// ParseCIDR formats the Range of the IPPool
func (i NodeSlicePool) ParseCIDR() (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(i.Spec.Range)
}

// +genclient
// +kubebuilder:object:root=true

// NodeSlicePool is the Schema for the nodesliceippools API
type NodeSlicePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSlicePoolSpec   `json:"spec,omitempty"`
	Status NodeSlicePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeSlicePoolList contains a list of NodeSlicePool
type NodeSlicePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSlicePool `json:"items"`
}

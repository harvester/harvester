package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OverlappingRangeIPReservationSpec defines the desired state of OverlappingRangeIPReservation
type OverlappingRangeIPReservationSpec struct {
	ContainerID string `json:"containerid,omitempty"`
	PodRef      string `json:"podref"`
	IfName      string `json:"ifname,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true

// OverlappingRangeIPReservation is the Schema for the OverlappingRangeIPReservations API
type OverlappingRangeIPReservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OverlappingRangeIPReservationSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OverlappingRangeIPReservationList contains a list of OverlappingRangeIPReservation
type OverlappingRangeIPReservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []OverlappingRangeIPReservation `json:"items"`
}

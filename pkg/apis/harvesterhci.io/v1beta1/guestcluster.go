package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type GuestClusterState string

const (
	GuestClusterInitialized GuestClusterState = "" // initialized
	GuestClusterCreated     GuestClusterState = "GuestClusterCreated"
)

var (
	GuestClusterConditionRunning      condition.Cond = "Running"
	GuestClusterConditionWaitDeleting condition.Cond = "WaitDeleting" // waiting to enter deleting status
	GuestClusterConditionDeleting     condition.Cond = "Deleting"     // all resources are being deleted
	GuestClusterConditionDeleted      condition.Cond = "Deleted"      // all resources have been deleted
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

type GuestCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GuestClusterSpec   `json:"spec,omitempty"`
	Status GuestClusterStatus `json:"status,omitempty"`
}

type GuestClusterSpec struct {
	// +optional
	Resources map[types.UID]ResourceInfo `json:"resources,omitempty"`
	// +optional
	Machines map[types.UID]ResourceInfo `json:"machines,omitempty"` // machines (VMs) are used to determin a guest cluster
	// +optional
	FirstObservedTime metav1.Time `json:"firstObservedTime,omitempty"`
	// +optional
	LastObservedTime *metav1.Time `json:"lastObservedTime,omitempty"`
	// +optional
	DeletionGracePeriodSeconds *int64 `json:"deletionGracePeriodSeconds,omitempty"`
	// +optional
	DeletionStartTime *metav1.Time `json:"deletionStartTime,omitempty"`
}

type GuestClusterStatus struct {
	// +optional
	Status GuestClusterState `json:"status,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type ResourceInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Deleted bool `json:"deleted,omitempty"`
}

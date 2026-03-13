package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=ipusage;ipusages,scope=Cluster
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:subresource:status

type IPPoolUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPoolUsageSpec   `json:"spec,omitempty"`
	Status IPPoolUsageStatus `json:"status,omitempty"`
}

type IPPoolUsageSpec struct {
	// CIDR defines the address pool managed by this resource.
	CIDR string `json:"cidr"`
	// ReservedIPCount preserves the first N allocatable IPs in the CIDR for special use.
	// +kubebuilder:default=8
	ReservedIPCount int `json:"reservedIPCount,omitempty"`
}

type IPPoolUsageStatus struct {
	// ReservedIPs lists the IPs currently preserved from normal allocation.
	ReservedIPs []string `json:"reservedIPs,omitempty"`
	// Allocations tracks which resource is currently using each allocated IP.
	Allocations map[string]IPPoolAllocation `json:"allocations,omitempty"`
	// Conditions reports the reconciliation status for the pool.
	Conditions []Condition `json:"conditions,omitempty"`
}

type IPPoolAllocation struct {
	// Resource identifies the current owner of the IP.
	Resource IPPoolUsageResourceRef `json:"resource"`
	// AllocatedAt is when this IP was first assigned to the current resource.
	AllocatedAt *metav1.Time `json:"allocatedAt,omitempty"`
	// LastUpdateTime is when the allocation entry was last reconciled.
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

type IPPoolUsageResourceRef struct {
	APIVersion string    `json:"apiVersion,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Namespace  string    `json:"namespace,omitempty"`
	Name       string    `json:"name,omitempty"`
	UID        types.UID `json:"uid,omitempty"`
}

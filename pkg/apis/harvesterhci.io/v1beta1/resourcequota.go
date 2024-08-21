package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=hrq;hrqs,scope=Namespaced
// +kubebuilder:printcolumn:name="NAMESPACE_TOTAL_SNAPSHOT_SIZE_QUOTA",type=string,JSONPath=`.spec.snapshotLimit.namespaceTotalSnapshotSizeQuota`
// +kubebuilder:printcolumn:name="NAMESPACE_TOTAL_SNAPSHOT_SIZE_USAGE",type=string,JSONPath=`.status.snapshotLimitStatus.namespaceTotalSnapshotSizeUsage`
// +kubebuilder:subresource:status

type ResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceQuotaSpec   `json:"spec,omitempty"`
	Status ResourceQuotaStatus `json:"status,omitempty"`
}

type ResourceQuotaSpec struct {
	// +kubebuilder:validation:Optional
	SnapshotLimit SnapshotLimit `json:"snapshotLimit,omitempty"`
}

type ResourceQuotaStatus struct {
	// +kubebuilder:validation:Optional
	SnapshotLimitStatus SnapshotLimitStatus `json:"snapshotLimitStatus,omitempty"`
}

type SnapshotLimit struct {
	// +kubebuilder:validation:Minimum=0
	NamespaceTotalSnapshotSizeQuota int64            `json:"namespaceTotalSnapshotSizeQuota,omitempty"`
	VMTotalSnapshotSizeQuota        map[string]int64 `json:"vmTotalSnapshotSizeQuota,omitempty"`
}

type SnapshotLimitStatus struct {
	NamespaceTotalSnapshotSizeUsage int64            `json:"namespaceTotalSnapshotSizeUsage,omitempty"`
	VMTotalSnapshotSizeUsage        map[string]int64 `json:"vmTotalSnapshotSizeUsage,omitempty"`
}

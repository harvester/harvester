package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +enum
type PVCBackupType string

const (
	PVCBackupLH PVCBackupType = "lh"
)

// +genclient
// +kubebuilder:resource:shortName=pvcbackup;pvcbackups,scope=Namespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="source",type="string",JSONPath=`.spec.source`
// +kubebuilder:printcolumn:name="Handle",type=string,JSONPath=`.status.handle`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.status.csiProvider`
// +kubebuilder:printcolumn:name="Success",type=boolean,JSONPath=`.status.success`

type PVCBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PVCBackupSpec   `json:"spec"`
	Status PVCBackupStatus `json:"status,omitempty"`
}

type PVCBackupSpec struct {
	// +kubebuilder:default=lh
	// +kubebuilder:validation:Enum=lh
	// +kubebuilder:validation:Required
	Type PVCBackupType `json:"type"`

	// +kubebuilder:validation:Required
	Source string `json:"source"`
}

type PVCBackupStatus struct {
	// +optional
	Handle string `json:"handle"`

	// +optional
	CSIProvider string `json:"csiProvider"`

	// +optional
	SourceSpec corev1.PersistentVolumeClaimSpec `json:"sourceSpec,omitempty"`

	// +optional
	Success bool `json:"success,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// +enum
type PVCRestoreType string

const (
	PVCRestoreLH PVCRestoreType = "lh"
)

// +genclient
// +kubebuilder:resource:shortName=pvcrestore;pvcrestores,scope=Namespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="from",type="string",JSONPath=`.spec.from`
// +kubebuilder:printcolumn:name="Success",type=boolean,JSONPath=`.status.success`

type PVCRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PVCRestoreSpec   `json:"spec"`
	Status PVCRestoreStatus `json:"status,omitempty"`
}

type PVCRestoreSpec struct {
	// +kubebuilder:default=lh
	// +kubebuilder:validation:Enum=lh
	// +kubebuilder:validation:Required
	Type PVCRestoreType `json:"type"`

	// +kubebuilder:validation:Required
	From string `json:"from"`
}

type PVCRestoreStatus struct {
	// +optional
	Success bool `json:"success,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

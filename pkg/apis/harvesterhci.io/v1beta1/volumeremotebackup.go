package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +enum
type VolumeRemoteBackupType string

const (
	VolumeRemoteBackupLH VolumeRemoteBackupType = "lh"
)

// +genclient
// +kubebuilder:resource:shortName=volumeremotebackup;volumeremotebackups,scope=Namespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="source",type="string",JSONPath=`.spec.source`
// +kubebuilder:printcolumn:name="Handle",type=string,JSONPath=`.status.handle`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.status.csiProvider`
// +kubebuilder:printcolumn:name="Success",type=boolean,JSONPath=`.status.success`

type VolumeRemoteBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeRemoteBackupSpec   `json:"spec"`
	Status VolumeRemoteBackupStatus `json:"status,omitempty"`
}

type VolumeRemoteBackupSpec struct {
	// +kubebuilder:default=lh
	// +kubebuilder:validation:Enum=lh
	// +kubebuilder:validation:Required
	Type VolumeRemoteBackupType `json:"type"`

	// +kubebuilder:validation:Required
	Source string `json:"source"`
}

type VolumeRemoteBackupStatus struct {
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
type VolumeRemoteRestoreType string

const (
	VolumeRemoteRestoreLH VolumeRemoteRestoreType = "lh"
)

// +genclient
// +kubebuilder:resource:shortName=volumeremoterestore;volumeremoterestores,scope=Namespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="type",type="string",JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="from",type="string",JSONPath=`.spec.from`
// +kubebuilder:printcolumn:name="Success",type=boolean,JSONPath=`.status.success`

type VolumeRemoteRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeRemoteRestoreSpec   `json:"spec"`
	Status VolumeRemoteRestoreStatus `json:"status,omitempty"`
}

type VolumeRemoteRestoreSpec struct {
	// +kubebuilder:default=lh
	// +kubebuilder:validation:Enum=lh
	// +kubebuilder:validation:Required
	Type VolumeRemoteRestoreType `json:"type"`

	// +kubebuilder:validation:Required
	From string `json:"from"`
}

type VolumeRemoteRestoreStatus struct {
	// +optional
	Success bool `json:"success,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

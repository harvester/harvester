package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var BackupSuspend condition.Cond = "BackupSuspend"

type VolumeBackupInfo struct {
	// +optional
	Name *string `json:"name,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`
}
type VMBackupInfo struct {
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	VolumeBackupInfo []VolumeBackupInfo `json:"volumeBackupInfo,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=svmbackup;svmbackups,scope=Namespaced
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cron`
// +kubebuilder:printcolumn:name="Retain",type=integer,JSONPath=`.spec.retain`
// +kubebuilder:printcolumn:name="MaxFailure",type=integer,JSONPath=`.spec.maxFailure`
// +kubebuilder:printcolumn:name="SpecSuspend",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.vmbackup.source.name`
// +kubebuilder:printcolumn:name="Suspended",type=string,JSONPath=`.status.suspended`
// +kubebuilder:printcolumn:name="Failure",type=integer,JSONPath=`.status.failure`

type ScheduleVMBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduleVMBackupSpec   `json:"spec"`
	Status ScheduleVMBackupStatus `json:"status,omitempty"`
}

type ScheduleVMBackupSpec struct {
	// +kubebuilder:validation:Required
	Cron string `json:"cron"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default:=8
	// +kubebuilder:validation:Maximum=250
	// +kubebuilder:validation:Minimum=2
	Retain int `json:"retain"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default:=4
	// +kubebuilder:validation:Minimum=2
	MaxFailure int `json:"maxFailure"`

	// +optional
	// +kubebuilder:default:=false
	Suspend bool `json:"suspend"`

	// +kubebuilder:validation:Required
	VMBackupSpec VirtualMachineBackupSpec `json:"vmbackup"`
}

type ScheduleVMBackupStatus struct {
	// +optional
	VMBackupInfo []VMBackupInfo `json:"vmbackupInfo,omitempty"`

	// +optional
	Failure int `json:"failure,omitempty"`

	// +optional
	Suspended bool `json:"suspended,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

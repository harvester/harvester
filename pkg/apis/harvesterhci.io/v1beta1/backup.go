package v1beta1

// NOTES: Harvester VM backup & restore is referenced from the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controllers because of the following issues:
// 1. live VM snapshot/backup should be supported, but it is prohibited on the Kubevirt side.
// 2. restore a VM backup to a new VM should be supported.
import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kv1 "kubevirt.io/client-go/api/v1"
)

const (
	// BackupConditionReady is the "ready" condition type
	BackupConditionReady condition.Cond = "Ready"

	// ConditionProgressing is the "progressing" condition type
	BackupConditionProgressing condition.Cond = "InProgress"
)

// DeletionPolicy defines that to do with resources when VirtualMachineRestore is deleted
type DeletionPolicy string

const (
	// VirtualMachineRestoreDelete is the default and causes the
	// VirtualMachineRestore deleted resources like dataVolume or PVC to be deleted
	VirtualMachineRestoreDelete DeletionPolicy = "delete"

	// VirtualMachineRestoreRetain causes the VirtualMachineRestore deleted resources like dataVolume or PVC to be retained
	VirtualMachineRestoreRetain DeletionPolicy = "retain"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmbackup;vmbackups,scope=Namespaced
// +kubebuilder:printcolumn:name="SOURCE_KIND",type=string,JSONPath=`.spec.source.kind`
// +kubebuilder:printcolumn:name="SOURCE_NAME",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="READY_TO_USE",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ERROR",type=date,JSONPath=`.status.error.message`

type VirtualMachineBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineBackupSpec `json:"spec"`

	// +optional
	Status *VirtualMachineBackupStatus `json:"status,omitempty"`
}

type VirtualMachineBackupSpec struct {
	Source corev1.TypedLocalObjectReference `json:"source"`
}

// VirtualMachineBackupStatus is the status for a VirtualMachineBackup resource
type VirtualMachineBackupStatus struct {
	// +optional
	SourceUID *types.UID `json:"sourceUID,omitempty"`

	// +optional
	VirtualMachineBackupContentName *string `json:"virtualMachineBackupContentName,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// Error is the last error encountered during the snapshot/restore
type Error struct {
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// +optional
	Message *string `json:"message,omitempty"`
}

// VirtualMachineBackupContent contains the snapshot data
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmbackupcontent;vmbackupcontents,scope=Namespaced
// +kubebuilder:printcolumn:name="READY_TO_USE",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ERROR",type=string,JSONPath=`.status.error.message`

type VirtualMachineBackupContent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineBackupContentSpec `json:"spec"`

	// +optional
	Status *VirtualMachineBackupContentStatus `json:"status,omitempty"`
}

// VirtualMachineBackupContentSpec is the spec for a VirtualMachineBackupContent resource
type VirtualMachineBackupContentSpec struct {
	VirtualMachineBackupName *string `json:"virtualMachineBackupName,omitempty"`

	Source SourceSpec `json:"source"`

	// +optional
	VolumeBackups []VolumeBackup `json:"volumeBackups,omitempty"`
}

// SourceSpec contains the appropriate spec for the resource being snapshotted
type SourceSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +optional
	VirtualMachineSpec *kv1.VirtualMachineSpec `json:"virtualMachineSpec,omitempty"`
}

// VolumeBackup contains the volume data need to restore a PVC
type VolumeBackup struct {
	VolumeName string `json:"volumeName"`

	PersistentVolumeClaim PersistentVolumeClaimSpec `json:"persistentVolumeClaim"`

	// +optional
	Name *string `json:"name,omitempty"`
}

type PersistentVolumeClaimSpec struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// +optional
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// VirtualMachineBackupContentStatus is the status for a VirtualMachineBackupStatus resource
type VirtualMachineBackupContentStatus struct {
	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`

	// +optional
	VolumeBackupStatus []VolumeBackupStatus `json:"volumeBackupStatus,omitempty"`
}

// VolumeBackupStatus is the status of a VolumeBackup
type VolumeBackupStatus struct {
	VolumeBackupName string `json:"volumeBackupName"`

	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`
}

// VirtualMachineRestore defines the operation of restoring a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmrestore;vmrestores,scope=Namespaced
// +kubebuilder:printcolumn:name="TARGET_KIND",type=string,JSONPath=`.spec.target.kind`
// +kubebuilder:printcolumn:name="TARGET_NAME",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="COMPLETE",type=boolean,JSONPath=`.status.complete`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ERROR",type=date,JSONPath=`.status.error.message`

type VirtualMachineRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineRestoreSpec `json:"spec"`

	// +optional
	Status *VirtualMachineRestoreStatus `json:"status,omitempty"`
}

// VirtualMachineRestoreSpec is the spec for a VirtualMachineRestore resource
type VirtualMachineRestoreSpec struct {
	// initially only VirtualMachine type supported
	Target corev1.TypedLocalObjectReference `json:"target"`

	VirtualMachineBackupName string `json:"virtualMachineBackupName"`

	// +optional
	NewVM bool `json:"newVM,omitempty"`

	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// VirtualMachineRestoreStatus is the spec for a VirtualMachineRestore resource
type VirtualMachineRestoreStatus struct {
	// +optional
	VolumeRestores []VolumeRestore `json:"restores,omitempty"`

	// +optional
	RestoreTime *metav1.Time `json:"restoreTime,omitempty"`

	// +optional
	DeletedDataVolumes []string `json:"deletedDataVolumes,omitempty"`

	// +optional
	Complete *bool `json:"complete,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	TargetUID *types.UID `json:"targetUID,omitempty"`
}

// VolumeRestore contains the volume data need to restore a PVC
type VolumeRestore struct {
	VolumeName string `json:"volumeName"`

	PersistentVolumeClaim PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec"`

	VolumeBackupName string `json:"volumeBackupName"`
}

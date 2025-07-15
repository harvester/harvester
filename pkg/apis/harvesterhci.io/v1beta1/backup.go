package v1beta1

// NOTES: Harvester VM backup & restore is referenced from the Kubevirt's VM snapshot & restore,
// currently, we have decided to use custom VM backup and restore controllers because of the following issues:
// 1. live VM snapshot/backup should be supported, but it is prohibited on the Kubevirt side.
// 2. restore a VM backup to a new VM should be supported.
import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// BackupConditionReady is the "ready" condition type
	BackupConditionReady condition.Cond = "Ready"

	// ConditionProgressing is the "progressing" condition type
	BackupConditionProgressing condition.Cond = "InProgress"

	// BackupConditionMetadataReady is the "metadataReady" condition type
	BackupConditionMetadataReady condition.Cond = "MetadataReady"
)

// DeletionPolicy defines that to do with resources when VirtualMachineRestore is deleted
type DeletionPolicy string

const (
	// VirtualMachineRestoreDelete is the default and causes the
	// VirtualMachineRestore deleted resources like PVC to be deleted
	VirtualMachineRestoreDelete DeletionPolicy = "delete"

	// VirtualMachineRestoreRetain causes the VirtualMachineRestore deleted resources like PVC to be retained
	VirtualMachineRestoreRetain DeletionPolicy = "retain"
)

type BackupType string

const (
	Backup   BackupType = "backup"
	Snapshot BackupType = "snapshot"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmbackup;vmbackups,scope=Namespaced
// +kubebuilder:printcolumn:name="SOURCE_KIND",type=string,JSONPath=`.spec.source.kind`
// +kubebuilder:printcolumn:name="SOURCE_NAME",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="READY_TO_USE",type=boolean,JSONPath=`.status.readyToUse`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="ERROR",type=date,JSONPath=`.status.error.message`

type VirtualMachineBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineBackupSpec `json:"spec"`

	// +optional
	Status *VirtualMachineBackupStatus `json:"status,omitempty" default:""`
}

type VirtualMachineBackupSpec struct {
	Source corev1.TypedLocalObjectReference `json:"source"`

	// +kubebuilder:default:="backup"
	// +kubebuilder:validation:Enum=backup;snapshot
	// +kubebuilder:validation:Optional
	Type BackupType `json:"type,omitempty" default:"backup"`
}

// VirtualMachineBackupStatus is the status for a VirtualMachineBackup resource
type VirtualMachineBackupStatus struct {
	// +optional
	SourceUID *types.UID `json:"sourceUID,omitempty"`

	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty"`

	// +optional
	BackupTarget *BackupTarget `json:"backupTarget,omitempty"`

	// +optional
	CSIDriverVolumeSnapshotClassNames map[string]string `json:"csiDriverVolumeSnapshotClassNames,omitempty"`

	// +kubebuilder:validation:Required
	// SourceSpec contains the vm spec source of the backup target
	SourceSpec *VirtualMachineSourceSpec `json:"source,omitempty"`

	// +optional
	VolumeBackups []VolumeBackup `json:"volumeBackups,omitempty"`

	// +optional
	SecretBackups []SecretBackup `json:"secretBackups,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// BackupTarget is where VM Backup stores
type BackupTarget struct {
	Endpoint     string `json:"endpoint,omitempty"`
	BucketName   string `json:"bucketName,omitempty"`
	BucketRegion string `json:"bucketRegion,omitempty"`
}

// Error is the last error encountered during the snapshot/restore
type Error struct {
	// +optional
	Time *metav1.Time `json:"time,omitempty"`

	// +optional
	Message *string `json:"message,omitempty"`
}

// VolumeBackup contains the volume data need to restore a PVC
type VolumeBackup struct {
	// +optional
	Name *string `json:"name,omitempty"`

	// +kubebuilder:validation:Required
	VolumeName string `json:"volumeName"`

	// +kubebuilder:default:="driver.longhorn.io"
	// +kubebuilder:validation:Required
	CSIDriverName string `json:"csiDriverName"`

	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty"`

	// +kubebuilder:validation:Required
	PersistentVolumeClaim PersistentVolumeClaimSourceSpec `json:"persistentVolumeClaim"`

	// +optional
	LonghornBackupName *string `json:"longhornBackupName,omitempty"`

	// +optional
	VolumeSize int64 `json:"volumeSize,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`

	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty"`

	// +optional
	Error *Error `json:"error,omitempty"`
}

// SecretBackup contains the secret data need to restore a secret referenced by the VM
type SecretBackup struct {
	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`

	// +optional
	Data map[string][]byte `json:"data,omitempty"`
}

type PersistentVolumeClaimSourceSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
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

	// +kubebuilder:validation:Required
	VirtualMachineBackupName string `json:"virtualMachineBackupName"`

	// +kubebuilder:validation:Required
	VirtualMachineBackupNamespace string `json:"virtualMachineBackupNamespace"`

	// +optional
	NewVM bool `json:"newVM,omitempty"`

	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// +optional
	// KeepMacAddress only works when NewVM is true.
	// For replacing original VM, the macaddress will be the same.
	KeepMacAddress bool `json:"keepMacAddress,omitempty"`
}

// VirtualMachineRestoreStatus is the spec for a VirtualMachineRestore resource
type VirtualMachineRestoreStatus struct {
	// +optional
	VolumeRestores []VolumeRestore `json:"restores,omitempty"`

	// +optional
	RestoreTime *metav1.Time `json:"restoreTime,omitempty"`

	// +optional
	DeletedVolumes []string `json:"deletedVolumes,omitempty"`

	// +optional
	Complete *bool `json:"complete,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	TargetUID *types.UID `json:"targetUID,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`
}

// VolumeRestore contains the volume data need to restore a PVC
type VolumeRestore struct {
	VolumeName string `json:"volumeName,omitempty"`

	PersistentVolumeClaim PersistentVolumeClaimSourceSpec `json:"persistentVolumeClaimSpec,omitempty"`

	VolumeBackupName string `json:"volumeBackupName,omitempty"`

	// +optional
	LonghornEngineName *string `json:"longhornEngineName,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`

	// +optional
	VolumeSize int64 `json:"volumeSize,omitempty"`
}

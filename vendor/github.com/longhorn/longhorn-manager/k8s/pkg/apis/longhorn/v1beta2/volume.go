package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type VolumeState string

const (
	VolumeStateCreating  = VolumeState("creating")
	VolumeStateAttached  = VolumeState("attached")
	VolumeStateDetached  = VolumeState("detached")
	VolumeStateAttaching = VolumeState("attaching")
	VolumeStateDetaching = VolumeState("detaching")
	VolumeStateDeleting  = VolumeState("deleting")
)

type VolumeRobustness string

const (
	VolumeRobustnessHealthy  = VolumeRobustness("healthy")  // during attached
	VolumeRobustnessDegraded = VolumeRobustness("degraded") // during attached
	VolumeRobustnessFaulted  = VolumeRobustness("faulted")  // during detached
	VolumeRobustnessUnknown  = VolumeRobustness("unknown")
)

// +kubebuilder:validation:Enum=blockdev;iscsi;""
type VolumeFrontend string

const (
	VolumeFrontendBlockDev = VolumeFrontend("blockdev")
	VolumeFrontendISCSI    = VolumeFrontend("iscsi")
	VolumeFrontendEmpty    = VolumeFrontend("")
)

type VolumeDataSource string

type VolumeDataSourceType string

const (
	VolumeDataSourceTypeBackup   = VolumeDataSourceType("backup") // Planing to move FromBackup field into DataSource field
	VolumeDataSourceTypeSnapshot = VolumeDataSourceType("snapshot")
	VolumeDataSourceTypeVolume   = VolumeDataSourceType("volume")
)

// +kubebuilder:validation:Enum=disabled;best-effort
type DataLocality string

const (
	DataLocalityDisabled   = DataLocality("disabled")
	DataLocalityBestEffort = DataLocality("best-effort")
)

// +kubebuilder:validation:Enum=rwo;rwx
type AccessMode string

const (
	AccessModeReadWriteOnce = AccessMode("rwo")
	AccessModeReadWriteMany = AccessMode("rwx")
)

// +kubebuilder:validation:Enum=ignored;disabled;least-effort;best-effort
type ReplicaAutoBalance string

const (
	ReplicaAutoBalanceIgnored     = ReplicaAutoBalance("ignored")
	ReplicaAutoBalanceDisabled    = ReplicaAutoBalance("disabled")
	ReplicaAutoBalanceLeastEffort = ReplicaAutoBalance("least-effort")
	ReplicaAutoBalanceBestEffort  = ReplicaAutoBalance("best-effort")
)

type VolumeCloneState string

const (
	VolumeCloneStateEmpty     = VolumeCloneState("")
	VolumeCloneStateInitiated = VolumeCloneState("initiated")
	VolumeCloneStateCompleted = VolumeCloneState("completed")
	VolumeCloneStateFailed    = VolumeCloneState("failed")
)

type VolumeCloneStatus struct {
	// +optional
	SourceVolume string `json:"sourceVolume"`
	// +optional
	Snapshot string `json:"snapshot"`
	// +optional
	State VolumeCloneState `json:"state"`
}

const (
	VolumeConditionTypeScheduled        = "scheduled"
	VolumeConditionTypeRestore          = "restore"
	VolumeConditionTypeTooManySnapshots = "toomanysnapshots"
)

const (
	VolumeConditionReasonReplicaSchedulingFailure      = "ReplicaSchedulingFailure"
	VolumeConditionReasonLocalReplicaSchedulingFailure = "LocalReplicaSchedulingFailure"
	VolumeConditionReasonRestoreInProgress             = "RestoreInProgress"
	VolumeConditionReasonRestoreFailure                = "RestoreFailure"
	VolumeConditionReasonTooManySnapshots              = "TooManySnapshots"
)

// VolumeRecurringJobSpec is a deprecated struct.
// TODO: Should be removed when recurringJobs gets removed from the volume
//       spec.
type VolumeRecurringJobSpec struct {
	// +optional
	Name string `json:"name"`
	// +optional
	Groups []string `json:"groups,omitempty"`
	// +optional
	Task RecurringJobType `json:"task"`
	// +optional
	Cron string `json:"cron"`
	// +optional
	Retain int `json:"retain"`
	// +optional
	Concurrency int `json:"concurrency"`
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

type KubernetesStatus struct {
	// +optional
	PVName string `json:"pvName"`
	// +optional
	PVStatus string `json:"pvStatus"`
	// determine if PVC/Namespace is history or not
	// +optional
	Namespace string `json:"namespace"`
	// +optional
	PVCName string `json:"pvcName"`
	// +optional
	LastPVCRefAt string `json:"lastPVCRefAt"`
	// determine if Pod/Workload is history or not
	// +optional
	// +nullable
	WorkloadsStatus []WorkloadStatus `json:"workloadsStatus"`
	// +optional
	LastPodRefAt string `json:"lastPodRefAt"`
}

type WorkloadStatus struct {
	// +optional
	PodName string `json:"podName"`
	// +optional
	PodStatus string `json:"podStatus"`
	// +optional
	WorkloadName string `json:"workloadName"`
	// +optional
	WorkloadType string `json:"workloadType"`
}

// VolumeSpec defines the desired state of the Longhorn volume
type VolumeSpec struct {
	// +kubebuilder:validation:Type=string
	// +optional
	Size int64 `json:"size,string"`
	// +optional
	Frontend VolumeFrontend `json:"frontend"`
	// +optional
	FromBackup string `json:"fromBackup"`
	// +optional
	DataSource VolumeDataSource `json:"dataSource"`
	// +optional
	DataLocality DataLocality `json:"dataLocality"`
	// +optional
	StaleReplicaTimeout int `json:"staleReplicaTimeout"`
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	MigrationNodeID string `json:"migrationNodeID"`
	// +optional
	EngineImage string `json:"engineImage"`
	// +optional
	BackingImage string `json:"backingImage"`
	// +optional
	Standby bool `json:"Standby"`
	// +optional
	DiskSelector []string `json:"diskSelector"`
	// +optional
	NodeSelector []string `json:"nodeSelector"`
	// +optional
	DisableFrontend bool `json:"disableFrontend"`
	// +optional
	RevisionCounterDisabled bool `json:"revisionCounterDisabled"`
	// +optional
	LastAttachedBy string `json:"lastAttachedBy"`
	// +optional
	AccessMode AccessMode `json:"accessMode"`
	// +optional
	Migratable bool `json:"migratable"`
	// +optional
	Encrypted bool `json:"encrypted"`
	// +optional
	NumberOfReplicas int `json:"numberOfReplicas"`
	// +optional
	ReplicaAutoBalance ReplicaAutoBalance `json:"replicaAutoBalance"`
	// Deprecated. Rename to BackingImage
	// +optional
	BaseImage string `json:"baseImage"`
	// Deprecated. Replaced by a separate resource named "RecurringJob"
	// +optional
	RecurringJobs []VolumeRecurringJobSpec `json:"recurringJobs,omitempty"`
}

// VolumeStatus defines the observed state of the Longhorn volume
type VolumeStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	State VolumeState `json:"state"`
	// +optional
	Robustness VolumeRobustness `json:"robustness"`
	// +optional
	CurrentNodeID string `json:"currentNodeID"`
	// +optional
	CurrentImage string `json:"currentImage"`
	// +optional
	KubernetesStatus KubernetesStatus `json:"kubernetesStatus"`
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
	// +optional
	LastBackup string `json:"lastBackup"`
	// +optional
	LastBackupAt string `json:"lastBackupAt"`
	// +optional
	PendingNodeID string `json:"pendingNodeID"`
	// +optional
	FrontendDisabled bool `json:"frontendDisabled"`
	// +optional
	RestoreRequired bool `json:"restoreRequired"`
	// +optional
	RestoreInitiated bool `json:"restoreInitiated"`
	// +optional
	CloneStatus VolumeCloneStatus `json:"cloneStatus"`
	// +optional
	RemountRequestedAt string `json:"remountRequestedAt"`
	// +optional
	ExpansionRequired bool `json:"expansionRequired"`
	// +optional
	IsStandby bool `json:"isStandby"`
	// +optional
	ActualSize int64 `json:"actualSize"`
	// +optional
	LastDegradedAt string `json:"lastDegradedAt"`
	// +optional
	ShareEndpoint string `json:"shareEndpoint"`
	// +optional
	ShareState ShareManagerState `json:"shareState"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhv
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The state of the volume"
// +kubebuilder:printcolumn:name="Robustness",type=string,JSONPath=`.status.robustness`,description="The robustness of the volume"
// +kubebuilder:printcolumn:name="Scheduled",type=string,JSONPath=`.status.conditions[?(@.type=='Schedulable')].status`,description="The scheduled condition of the volume"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`,description="The size of the volume"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.currentNodeID`,description="The node that the volume is currently attaching to"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Volume is where Longhorn stores volume object.
type Volume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeSpec   `json:"spec,omitempty"`
	Status VolumeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeList is a list of Volumes.
type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Volume `json:"items"`
}

// Hub defines the current version (v1beta2) is the storage version
// so mark this as Hub
func (v *Volume) Hub() {}

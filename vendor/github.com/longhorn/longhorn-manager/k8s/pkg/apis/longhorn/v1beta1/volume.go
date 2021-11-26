package v1beta1

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

type VolumeFrontend string

const (
	VolumeFrontendBlockDev = VolumeFrontend("blockdev")
	VolumeFrontendISCSI    = VolumeFrontend("iscsi")
	VolumeFrontendEmpty    = VolumeFrontend("")
)

type VolumeDataSource string

const (
	VolumeDataSourceTypeBackup   = "backup" // Planing to move FromBackup field into DataSource field
	VolumeDataSourceTypeSnapshot = "snapshot"
	VolumeDataSourceTypeVolume   = "volume"
)

type DataLocality string

const (
	DataLocalityDisabled   = DataLocality("disabled")
	DataLocalityBestEffort = DataLocality("best-effort")
)

type AccessMode string

const (
	AccessModeReadWriteOnce = AccessMode("rwo")
	AccessModeReadWriteMany = AccessMode("rwx")
)

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
	SourceVolume string           `json:"sourceVolume"`
	Snapshot     string           `json:"snapshot"`
	State        VolumeCloneState `json:"state"`
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
	Name        string            `json:"name"`
	Groups      []string          `json:"groups,omitempty"`
	Task        RecurringJobType  `json:"task"`
	Cron        string            `json:"cron"`
	Retain      int               `json:"retain"`
	Concurrency int               `json:"concurrency"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type KubernetesStatus struct {
	PVName   string `json:"pvName"`
	PVStatus string `json:"pvStatus"`

	// determine if PVC/Namespace is history or not
	Namespace    string `json:"namespace"`
	PVCName      string `json:"pvcName"`
	LastPVCRefAt string `json:"lastPVCRefAt"`

	// determine if Pod/Workload is history or not
	WorkloadsStatus []WorkloadStatus `json:"workloadsStatus"`
	LastPodRefAt    string           `json:"lastPodRefAt"`
}

type WorkloadStatus struct {
	PodName      string `json:"podName"`
	PodStatus    string `json:"podStatus"`
	WorkloadName string `json:"workloadName"`
	WorkloadType string `json:"workloadType"`
}

type VolumeSpec struct {
	Size                    int64            `json:"size,string"`
	Frontend                VolumeFrontend   `json:"frontend"`
	FromBackup              string           `json:"fromBackup"`
	DataSource              VolumeDataSource `json:"dataSource"`
	DataLocality            DataLocality     `json:"dataLocality"`
	StaleReplicaTimeout     int              `json:"staleReplicaTimeout"`
	NodeID                  string           `json:"nodeID"`
	MigrationNodeID         string           `json:"migrationNodeID"`
	EngineImage             string           `json:"engineImage"`
	BackingImage            string           `json:"backingImage"`
	Standby                 bool             `json:"Standby"`
	DiskSelector            []string         `json:"diskSelector"`
	NodeSelector            []string         `json:"nodeSelector"`
	DisableFrontend         bool             `json:"disableFrontend"`
	RevisionCounterDisabled bool             `json:"revisionCounterDisabled"`
	LastAttachedBy          string           `json:"lastAttachedBy"`
	AccessMode              AccessMode       `json:"accessMode"`
	Migratable              bool             `json:"migratable"`

	Encrypted bool `json:"encrypted"`

	NumberOfReplicas   int                `json:"numberOfReplicas"`
	ReplicaAutoBalance ReplicaAutoBalance `json:"replicaAutoBalance"`

	// Deprecated. Rename to BackingImage
	BaseImage string `json:"baseImage"`

	// Deprecated. Replaced by a separate resource named "RecurringJob"
	RecurringJobs []VolumeRecurringJobSpec `json:"recurringJobs,omitempty"`
}

type VolumeStatus struct {
	OwnerID            string               `json:"ownerID"`
	State              VolumeState          `json:"state"`
	Robustness         VolumeRobustness     `json:"robustness"`
	CurrentNodeID      string               `json:"currentNodeID"`
	CurrentImage       string               `json:"currentImage"`
	KubernetesStatus   KubernetesStatus     `json:"kubernetesStatus"`
	Conditions         map[string]Condition `json:"conditions"`
	LastBackup         string               `json:"lastBackup"`
	LastBackupAt       string               `json:"lastBackupAt"`
	PendingNodeID      string               `json:"pendingNodeID"`
	FrontendDisabled   bool                 `json:"frontendDisabled"`
	RestoreRequired    bool                 `json:"restoreRequired"`
	RestoreInitiated   bool                 `json:"restoreInitiated"`
	CloneStatus        VolumeCloneStatus    `json:"cloneStatus"`
	RemountRequestedAt string               `json:"remountRequestedAt"`
	ExpansionRequired  bool                 `json:"expansionRequired"`
	IsStandby          bool                 `json:"isStandby"`
	ActualSize         int64                `json:"actualSize"`
	LastDegradedAt     string               `json:"lastDegradedAt"`
	ShareEndpoint      string               `json:"shareEndpoint"`
	ShareState         ShareManagerState    `json:"shareState"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Volume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              VolumeSpec   `json:"spec"`
	Status            VolumeStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Volume `json:"items"`
}

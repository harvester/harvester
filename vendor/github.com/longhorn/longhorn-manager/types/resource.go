package types

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

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

type Condition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	LastProbeTime      string          `json:"lastProbeTime"`
	LastTransitionTime string          `json:"lastTransitionTime"`
	Reason             string          `json:"reason"`
	Message            string          `json:"message"`
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

type VolumeSpec struct {
	Size                    int64          `json:"size,string"`
	Frontend                VolumeFrontend `json:"frontend"`
	FromBackup              string         `json:"fromBackup"`
	NumberOfReplicas        int            `json:"numberOfReplicas"`
	DataLocality            DataLocality   `json:"dataLocality"`
	StaleReplicaTimeout     int            `json:"staleReplicaTimeout"`
	NodeID                  string         `json:"nodeID"`
	EngineImage             string         `json:"engineImage"`
	RecurringJobs           []RecurringJob `json:"recurringJobs"`
	BaseImage               string         `json:"baseImage"`
	Standby                 bool           `json:"Standby"`
	DiskSelector            []string       `json:"diskSelector"`
	NodeSelector            []string       `json:"nodeSelector"`
	DisableFrontend         bool           `json:"disableFrontend"`
	RevisionCounterDisabled bool           `json:"revisionCounterDisabled"`
	LastAttachedBy          string         `json:"lastAttachedBy"`
	AccessMode              AccessMode     `json:"accessMode"`
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
	RemountRequestedAt string               `json:"remountRequestedAt"`
	ExpansionRequired  bool                 `json:"expansionRequired"`
	IsStandby          bool                 `json:"isStandby"`
	ActualSize         int64                `json:"actualSize"`
	LastDegradedAt     string               `json:"lastDegradedAt"`
	ShareEndpoint      string               `json:"shareEndpoint"`
	ShareState         ShareManagerState    `json:"shareState"`
}

type RecurringJobType string

const (
	RecurringJobTypeSnapshot = RecurringJobType("snapshot")
	RecurringJobTypeBackup   = RecurringJobType("backup")
)

type RecurringJob struct {
	Name   string            `json:"name"`
	Task   RecurringJobType  `json:"task"`
	Cron   string            `json:"cron"`
	Retain int               `json:"retain"`
	Labels map[string]string `json:"labels"`
}

type InstanceState string

const (
	InstanceStateRunning  = InstanceState("running")
	InstanceStateStopped  = InstanceState("stopped")
	InstanceStateError    = InstanceState("error")
	InstanceStateStarting = InstanceState("starting")
	InstanceStateStopping = InstanceState("stopping")
	InstanceStateUnknown  = InstanceState("unknown")
)

type InstanceSpec struct {
	VolumeName       string        `json:"volumeName"`
	VolumeSize       int64         `json:"volumeSize,string"`
	NodeID           string        `json:"nodeID"`
	EngineImage      string        `json:"engineImage"`
	DesireState      InstanceState `json:"desireState"`
	LogRequested     bool          `json:"logRequested"`
	SalvageRequested bool          `json:"salvageRequested"`
}

type InstanceStatus struct {
	OwnerID             string        `json:"ownerID"`
	InstanceManagerName string        `json:"instanceManagerName"`
	CurrentState        InstanceState `json:"currentState"`
	CurrentImage        string        `json:"currentImage"`
	IP                  string        `json:"ip"`
	Port                int           `json:"port"`
	Started             bool          `json:"started"`
	LogFetched          bool          `json:"logFetched"`
	SalvageExecuted     bool          `json:"salvageExecuted"`
}

type EngineSpec struct {
	InstanceSpec
	Frontend                  VolumeFrontend    `json:"frontend"`
	ReplicaAddressMap         map[string]string `json:"replicaAddressMap"`
	UpgradedReplicaAddressMap map[string]string `json:"upgradedReplicaAddressMap"`
	BackupVolume              string            `json:"backupVolume"`
	RequestedBackupRestore    string            `json:"requestedBackupRestore"`
	DisableFrontend           bool              `json:"disableFrontend"`
	RevisionCounterDisabled   bool              `json:"revisionCounterDisabled"`
}

type EngineStatus struct {
	InstanceStatus
	CurrentSize              int64                     `json:"currentSize,string"`
	CurrentReplicaAddressMap map[string]string         `json:"currentReplicaAddressMap"`
	ReplicaModeMap           map[string]ReplicaMode    `json:"replicaModeMap"`
	Endpoint                 string                    `json:"endpoint"`
	LastRestoredBackup       string                    `json:"lastRestoredBackup"`
	BackupStatus             map[string]*BackupStatus  `json:"backupStatus"`
	RestoreStatus            map[string]*RestoreStatus `json:"restoreStatus"`
	PurgeStatus              map[string]*PurgeStatus   `json:"purgeStatus"`
	RebuildStatus            map[string]*RebuildStatus `json:"rebuildStatus"`
	Snapshots                map[string]*Snapshot      `json:"snapshots"`
	SnapshotsError           string                    `json:"snapshotsError"`
	IsExpanding              bool                      `json:"isExpanding"`
	LastExpansionError       string                    `json:"lastExpansionError"`
	LastExpansionFailedAt    string                    `json:"lastExpansionFailedAt"`
}

type Snapshot struct {
	Name        string            `json:"name"`
	Parent      string            `json:"parent"`
	Children    map[string]bool   `json:"children"`
	Removed     bool              `json:"removed"`
	UserCreated bool              `json:"usercreated"`
	Created     string            `json:"created"`
	Size        string            `json:"size"`
	Labels      map[string]string `json:"labels"`
}

type ReplicaSpec struct {
	InstanceSpec
	EngineName              string `json:"engineName"`
	HealthyAt               string `json:"healthyAt"`
	FailedAt                string `json:"failedAt"`
	DiskID                  string `json:"diskID"`
	DiskPath                string `json:"diskPath"`
	DataDirectoryName       string `json:"dataDirectoryName"`
	BaseImage               string `json:"baseImage"`
	Active                  bool   `json:"active"`
	HardNodeAffinity        string `json:"hardNodeAffinity"`
	RevisionCounterDisabled bool   `json:"revisionCounterDisabled"`
	RebuildRetryCount       int    `json:"rebuildRetryCount"`

	// Deprecated
	DataPath string `json:"dataPath"`
}

type ReplicaStatus struct {
	InstanceStatus
	EvictionRequested bool `json:"evictionRequested"`
}

type EngineImageState string

const (
	EngineImageStateDeploying    = "deploying"
	EngineImageStateReady        = "ready"
	EngineImageStateIncompatible = "incompatible"
	EngineImageStateError        = "error"
)

type EngineImageSpec struct {
	Image string `json:"image"`
}

const (
	EngineImageConditionTypeReady = "ready"

	EngineImageConditionTypeReadyReasonDaemonSet = "daemonSet"
	EngineImageConditionTypeReadyReasonBinary    = "binary"
)

type EngineImageStatus struct {
	OwnerID    string               `json:"ownerID"`
	State      EngineImageState     `json:"state"`
	RefCount   int                  `json:"refCount"`
	NoRefSince string               `json:"noRefSince"`
	Conditions map[string]Condition `json:"conditions"`

	EngineVersionDetails
}

const (
	InvalidEngineVersion = -1
)

type EngineVersionDetails struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`

	CLIAPIVersion           int `json:"cliAPIVersion"`
	CLIAPIMinVersion        int `json:"cliAPIMinVersion"`
	ControllerAPIVersion    int `json:"controllerAPIVersion"`
	ControllerAPIMinVersion int `json:"controllerAPIMinVersion"`
	DataFormatVersion       int `json:"dataFormatVersion"`
	DataFormatMinVersion    int `json:"dataFormatMinVersion"`
}

type NodeSpec struct {
	Name              string              `json:"name"`
	Disks             map[string]DiskSpec `json:"disks"`
	AllowScheduling   bool                `json:"allowScheduling"`
	EvictionRequested bool                `json:"evictionRequested"`
	Tags              []string            `json:"tags"`
}

const (
	NodeConditionTypeReady            = "Ready"
	NodeConditionTypeMountPropagation = "MountPropagation"
	NodeConditionTypeSchedulable      = "Schedulable"
)

const (
	NodeConditionReasonManagerPodDown            = "ManagerPodDown"
	NodeConditionReasonManagerPodMissing         = "ManagerPodMissing"
	NodeConditionReasonKubernetesNodeGone        = "KubernetesNodeGone"
	NodeConditionReasonKubernetesNodeNotReady    = "KubernetesNodeNotReady"
	NodeConditionReasonKubernetesNodePressure    = "KubernetesNodePressure"
	NodeConditionReasonUnknownNodeConditionTrue  = "UnknownNodeConditionTrue"
	NodeConditionReasonNoMountPropagationSupport = "NoMountPropagationSupport"
	NodeConditionReasonKubernetesNodeCordoned    = "KubernetesNodeCordoned"
)

const (
	DiskConditionTypeSchedulable = "Schedulable"
	DiskConditionTypeReady       = "Ready"
)

const (
	DiskConditionReasonDiskPressure          = "DiskPressure"
	DiskConditionReasonDiskFilesystemChanged = "DiskFilesystemChanged"
	DiskConditionReasonNoDiskInfo            = "NoDiskInfo"
	DiskConditionReasonDiskNotReady          = "DiskNotReady"
)

type NodeStatus struct {
	Conditions map[string]Condition   `json:"conditions"`
	DiskStatus map[string]*DiskStatus `json:"diskStatus"`
	Region     string                 `json:"region"`
	Zone       string                 `json:"zone"`
}

type DiskSpec struct {
	Path              string   `json:"path"`
	AllowScheduling   bool     `json:"allowScheduling"`
	EvictionRequested bool     `json:"evictionRequested"`
	StorageReserved   int64    `json:"storageReserved"`
	Tags              []string `json:"tags"`
}

type DiskStatus struct {
	Conditions       map[string]Condition `json:"conditions"`
	StorageAvailable int64                `json:"storageAvailable"`
	StorageScheduled int64                `json:"storageScheduled"`
	StorageMaximum   int64                `json:"storageMaximum"`
	ScheduledReplica map[string]int64     `json:"scheduledReplica"`
	DiskUUID         string               `json:"diskUUID"`
}

type BackupStatus struct {
	Progress       int    `json:"progress"`
	BackupURL      string `json:"backupURL,omitempty"`
	Error          string `json:"error,omitempty"`
	SnapshotName   string `json:"snapshotName"`
	State          string `json:"state"`
	ReplicaAddress string `json:"replicaAddress"`
}

type RestoreStatus struct {
	IsRestoring            bool   `json:"isRestoring"`
	LastRestored           string `json:"lastRestored"`
	CurrentRestoringBackup string `json:"currentRestoringBackup"`
	Progress               int    `json:"progress,omitempty"`
	Error                  string `json:"error,omitempty"`
	Filename               string `json:"filename,omitempty"`
	State                  string `json:"state"`
	BackupURL              string `json:"backupURL"`
}

type PurgeStatus struct {
	Error     string `json:"error"`
	IsPurging bool   `json:"isPurging"`
	Progress  int    `json:"progress"`
	State     string `json:"state"`
}

type RebuildStatus struct {
	Error              string `json:"error"`
	IsRebuilding       bool   `json:"isRebuilding"`
	Progress           int    `json:"progress"`
	State              string `json:"state"`
	FromReplicaAddress string `json:"fromReplicaAddress"`
}

type InstanceType string

const (
	InstanceTypeEngine  = InstanceType("engine")
	InstanceTypeReplica = InstanceType("replica")
)

type InstanceManagerState string

const (
	InstanceManagerStateError    = InstanceManagerState("error")
	InstanceManagerStateRunning  = InstanceManagerState("running")
	InstanceManagerStateStopped  = InstanceManagerState("stopped")
	InstanceManagerStateStarting = InstanceManagerState("starting")
	InstanceManagerStateUnknown  = InstanceManagerState("unknown")
)

type InstanceManagerType string

const (
	InstanceManagerTypeEngine  = InstanceManagerType("engine")
	InstanceManagerTypeReplica = InstanceManagerType("replica")
)

type InstanceManagerSpec struct {
	Image  string              `json:"image"`
	NodeID string              `json:"nodeID"`
	Type   InstanceManagerType `json:"type"`

	// TODO: deprecate this field
	EngineImage string `json:"engineImage"`
}

type InstanceManagerStatus struct {
	OwnerID       string                     `json:"ownerID"`
	CurrentState  InstanceManagerState       `json:"currentState"`
	Instances     map[string]InstanceProcess `json:"instances"`
	IP            string                     `json:"ip"`
	APIMinVersion int                        `json:"apiMinVersion"`
	APIVersion    int                        `json:"apiVersion"`
}

type InstanceProcess struct {
	Spec   InstanceProcessSpec   `json:"spec"`
	Status InstanceProcessStatus `json:"status"`
}

type InstanceProcessSpec struct {
	Name string `json:"name"`
}

type InstanceProcessStatus struct {
	Endpoint        string        `json:"endpoint"`
	ErrorMsg        string        `json:"errorMsg"`
	Listen          string        `json:"listen"`
	PortEnd         int32         `json:"portEnd"`
	PortStart       int32         `json:"portStart"`
	State           InstanceState `json:"state"`
	Type            InstanceType  `json:"type"`
	ResourceVersion int64         `json:"resourceVersion"`
}

type ShareManagerState string

const (
	ShareManagerStateUnknown  = ShareManagerState("unknown")
	ShareManagerStateStarting = ShareManagerState("starting")
	ShareManagerStateRunning  = ShareManagerState("running")
	ShareManagerStateStopped  = ShareManagerState("stopped")
	ShareManagerStateError    = ShareManagerState("error")
)

type ShareManagerSpec struct {
	Image string `json:"image"`
}

type ShareManagerStatus struct {
	OwnerID  string            `json:"ownerID"`
	State    ShareManagerState `json:"state"`
	Endpoint string            `json:"endpoint"`
}

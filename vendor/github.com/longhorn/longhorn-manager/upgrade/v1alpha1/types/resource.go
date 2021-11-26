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

type VolumeConditionType string

const (
	VolumeConditionTypeScheduled = "scheduled"
)

const (
	VolumeConditionReasonReplicaSchedulingFailure = "ReplicaSchedulingFailure"
)

type VolumeSpec struct {
	OwnerID                    string         `json:"ownerID"`
	Size                       int64          `json:"size,string"`
	Frontend                   VolumeFrontend `json:"frontend"`
	FromBackup                 string         `json:"fromBackup"`
	NumberOfReplicas           int            `json:"numberOfReplicas"`
	StaleReplicaTimeout        int            `json:"staleReplicaTimeout"`
	NodeID                     string         `json:"nodeID"`
	MigrationNodeID            string         `json:"migrationNodeID"`
	PendingNodeID              string         `json:"pendingNodeID"`
	EngineImage                string         `json:"engineImage"`
	RecurringJobs              []RecurringJob `json:"recurringJobs"`
	BaseImage                  string         `json:"baseImage"`
	Standby                    bool           `json:"Standby"`
	InitialRestorationRequired bool           `json:"initialRestorationRequired"`
	DiskSelector               []string       `json:"diskSelector"`
	NodeSelector               []string       `json:"nodeSelector"`
	DisableFrontend            bool           `json:"disableFrontend"`
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
	State      VolumeState      `json:"state"`
	Robustness VolumeRobustness `json:"robustness"`

	CurrentImage     string                            `json:"currentImage"`
	KubernetesStatus KubernetesStatus                  `json:"kubernetesStatus"`
	Conditions       map[VolumeConditionType]Condition `json:"conditions"`
	LastBackup       string                            `json:"lastBackup"`
	LastBackupAt     string                            `json:"lastBackupAt"`
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
)

type InstanceSpec struct {
	OwnerID      string        `json:"ownerID"`
	VolumeName   string        `json:"volumeName"`
	VolumeSize   int64         `json:"volumeSize,string"`
	NodeID       string        `json:"nodeID"`
	EngineImage  string        `json:"engineImage"`
	DesireState  InstanceState `json:"desireState"`
	LogRequested bool          `json:"logRequested"`
}

type InstanceStatus struct {
	InstanceManagerName string        `json:"instanceManagerName"`
	CurrentState        InstanceState `json:"currentState"`
	CurrentImage        string        `json:"currentImage"`
	IP                  string        `json:"ip"`
	Port                int           `json:"port"`
	Started             bool          `json:"started"`
	NodeBootID          string        `json:"nodeBootID"`
}

type EngineSpec struct {
	InstanceSpec
	Frontend                  VolumeFrontend    `json:"frontend"`
	ReplicaAddressMap         map[string]string `json:"replicaAddressMap"`
	UpgradedReplicaAddressMap map[string]string `json:"upgradedReplicaAddressMap"`
	BackupVolume              string            `json:"backupVolume"`
	RequestedBackupRestore    string            `json:"requestedBackupRestore"`
	DisableFrontend           bool              `json:"disableFrontend"`
}

type EngineStatus struct {
	InstanceStatus
	ReplicaModeMap     map[string]ReplicaMode    `json:"replicaModeMap"`
	Endpoint           string                    `json:"endpoint"`
	LastRestoredBackup string                    `json:"lastRestoredBackup"`
	BackupStatus       map[string]*BackupStatus  `json:"backupStatus"`
	RestoreStatus      map[string]*RestoreStatus `json:"restoreStatus"`
	PurgeStatus        map[string]*PurgeStatus   `json:"purgeStatus"`
}

type ReplicaSpec struct {
	InstanceSpec
	EngineName string `json:"engineName"`
	HealthyAt  string `json:"healthyAt"`
	FailedAt   string `json:"failedAt"`
	DiskID     string `json:"diskID"`
	DataPath   string `json:"dataPath"`
	BaseImage  string `json:"baseImage"`
	Active     bool   `json:"active"`
}

type ReplicaStatus struct {
	InstanceStatus
}

type EngineImageState string

const (
	EngineImageStateDeploying    = "deploying"
	EngineImageStateReady        = "ready"
	EngineImageStateIncompatible = "incompatible"
	EngineImageStateError        = "error"
)

type EngineImageSpec struct {
	OwnerID string `json:"ownerID"`
	Image   string `json:"image"`
}

type EngineImageStatus struct {
	State      EngineImageState `json:"state"`
	RefCount   int              `json:"refCount"`
	NoRefSince string           `json:"noRefSince"`

	EngineVersionDetails
}

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
	Name            string              `json:"name"`
	Disks           map[string]DiskSpec `json:"disks"`
	AllowScheduling bool                `json:"allowScheduling"`
	Tags            []string            `json:"tags"`
}

type NodeConditionType string

const (
	NodeConditionTypeReady            = "Ready"
	NodeConditionTypeMountPropagation = "MountPropagation"
)

const (
	NodeConditionReasonManagerPodDown            = "ManagerPodDown"
	NodeConditionReasonManagerPodMissing         = "ManagerPodMissing"
	NodeConditionReasonKubernetesNodeGone        = "KubernetesNodeGone"
	NodeConditionReasonKubernetesNodeNotReady    = "KubernetesNodeNotReady"
	NodeConditionReasonKubernetesNodePressure    = "KubernetesNodePressure"
	NodeConditionReasonUnknownNodeConditionTrue  = "UnknownNodeConditionTrue"
	NodeConditionReasonNoMountPropagationSupport = "NoMountPropagationSupport"
)

type DiskConditionType string

const (
	DiskConditionTypeSchedulable = "Schedulable"
	DiskConditionTypeReady       = "Ready"
)

const (
	DiskConditionReasonDiskPressure          = "DiskPressure"
	DiskConditionReasonDiskFilesystemChanged = "DiskFilesystemChanged"
	DiskConditionReasonNoDiskInfo            = "NoDiskInfo"
)

type NodeStatus struct {
	Conditions map[NodeConditionType]Condition `json:"conditions"`
	DiskStatus map[string]DiskStatus           `json:"diskStatus"`
}

type DiskSpec struct {
	Path            string   `json:"path"`
	AllowScheduling bool     `json:"allowScheduling"`
	StorageReserved int64    `json:"storageReserved"`
	Tags            []string `json:"tags"`
}

type DiskStatus struct {
	Conditions       map[DiskConditionType]Condition `json:"conditions"`
	StorageAvailable int64                           `json:"storageAvailable"`
	StorageScheduled int64                           `json:"storageScheduled"`
	StorageMaximum   int64                           `json:"storageMaximum"`
	ScheduledReplica map[string]int64                `json:"scheduledReplica"`
}

type BackupStatus struct {
	Progress     int    `json:"progress"`
	BackupURL    string `json:"backupURL,omitempty"`
	Error        string `json:"error,omitempty"`
	SnapshotName string `json:"snapshotName"`
	State        string `json:"state"`
}

type RestoreStatus struct {
	IsRestoring  bool   `json:"isRestoring"`
	LastRestored string `json:"lastRestored"`
	Progress     int    `json:"progress,omitempty"`
	Error        string `json:"error,omitempty"`
	Filename     string `json:"filename,omitempty"`
	State        string `json:"state"`
	BackupURL    string `json:"backupURL"`
}

type PurgeStatus struct {
	Error     string `json:"error"`
	IsPurging bool   `json:"isPurging"`
	Progress  int    `json:"progress"`
	State     string `json:"state"`
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
	EngineImage string              `json:"engineImage"`
	NodeID      string              `json:"nodeID"`
	OwnerID     string              `json:"ownerID"`
	Type        InstanceManagerType `json:"type"`
}

type InstanceManagerStatus struct {
	CurrentState InstanceManagerState       `json:"currentState"`
	Instances    map[string]InstanceProcess `json:"instances"`
	IP           string                     `json:"ip"`
	NodeBootID   string                     `json:"nodeBootID"`
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

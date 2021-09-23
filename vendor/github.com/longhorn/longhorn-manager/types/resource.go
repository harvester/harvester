package types

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

type ReplicaAutoBalance string

const (
	ReplicaAutoBalanceIgnored     = ReplicaAutoBalance("ignored")
	ReplicaAutoBalanceDisabled    = ReplicaAutoBalance("disabled")
	ReplicaAutoBalanceLeastEffort = ReplicaAutoBalance("least-effort")
	ReplicaAutoBalanceBestEffort  = ReplicaAutoBalance("best-effort")
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
	RecurringJobs []RecurringJob `json:"recurringJobs,omitempty"`
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
	CloneStatus        VolumeCloneStatus    `json:"cloneStatus"`
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

	RecurringJobGroupDefault = "default"
)

// RecurringJob is a deprecated struct.
// TODO: Should be removed when recurringJobs gets removed from the volume
//       spec.
type RecurringJob struct {
	Name        string            `json:"name"`
	Groups      []string          `json:"groups,omitempty"`
	Task        RecurringJobType  `json:"task"`
	Cron        string            `json:"cron"`
	Retain      int               `json:"retain"`
	Concurrency int               `json:"concurrency"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type VolumeRecurringJob struct {
	Name    string `json:"name"`
	IsGroup bool   `json:"isGroup"`
}

type VolumeCloneStatus struct {
	SourceVolume string           `json:"sourceVolume"`
	Snapshot     string           `json:"snapshot"`
	State        VolumeCloneState `json:"state"`
}

type VolumeCloneState string

const (
	VolumeCloneStateEmpty     = VolumeCloneState("")
	VolumeCloneStateInitiated = VolumeCloneState("initiated")
	VolumeCloneStateCompleted = VolumeCloneState("completed")
	VolumeCloneStateFailed    = VolumeCloneState("failed")
)

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
	RequestedDataSource       VolumeDataSource  `json:"requestedDataSource"`
	DisableFrontend           bool              `json:"disableFrontend"`
	RevisionCounterDisabled   bool              `json:"revisionCounterDisabled"`
}

type EngineStatus struct {
	InstanceStatus
	CurrentSize              int64                           `json:"currentSize,string"`
	CurrentReplicaAddressMap map[string]string               `json:"currentReplicaAddressMap"`
	ReplicaModeMap           map[string]ReplicaMode          `json:"replicaModeMap"`
	Endpoint                 string                          `json:"endpoint"`
	LastRestoredBackup       string                          `json:"lastRestoredBackup"`
	BackupStatus             map[string]*BackupStatus        `json:"backupStatus"`
	RestoreStatus            map[string]*RestoreStatus       `json:"restoreStatus"`
	PurgeStatus              map[string]*PurgeStatus         `json:"purgeStatus"`
	RebuildStatus            map[string]*RebuildStatus       `json:"rebuildStatus"`
	CloneStatus              map[string]*SnapshotCloneStatus `json:"cloneStatus"`
	Snapshots                map[string]*Snapshot            `json:"snapshots"`
	SnapshotsError           string                          `json:"snapshotsError"`
	IsExpanding              bool                            `json:"isExpanding"`
	LastExpansionError       string                          `json:"lastExpansionError"`
	LastExpansionFailedAt    string                          `json:"lastExpansionFailedAt"`
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
	BackingImage            string `json:"backingImage"`
	Active                  bool   `json:"active"`
	HardNodeAffinity        string `json:"hardNodeAffinity"`
	RevisionCounterDisabled bool   `json:"revisionCounterDisabled"`
	RebuildRetryCount       int    `json:"rebuildRetryCount"`

	// Deprecated
	DataPath string `json:"dataPath"`
	// Deprecated. Rename to BackingImage
	BaseImage string `json:"baseImage"`
}

type ReplicaStatus struct {
	InstanceStatus
	EvictionRequested bool `json:"evictionRequested"`
}

type EngineImageState string

const (
	EngineImageStateDeploying    = "deploying"
	EngineImageStateDeployed     = "deployed"
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
	OwnerID           string               `json:"ownerID"`
	State             EngineImageState     `json:"state"`
	RefCount          int                  `json:"refCount"`
	NoRefSince        string               `json:"noRefSince"`
	Conditions        map[string]Condition `json:"conditions"`
	NodeDeploymentMap map[string]bool      `json:"nodeDeploymentMap"`

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
	Name                     string              `json:"name"`
	Disks                    map[string]DiskSpec `json:"disks"`
	AllowScheduling          bool                `json:"allowScheduling"`
	EvictionRequested        bool                `json:"evictionRequested"`
	Tags                     []string            `json:"tags"`
	EngineManagerCPURequest  int                 `json:"engineManagerCPURequest"`
	ReplicaManagerCPURequest int                 `json:"replicaManagerCPURequest"`
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

type SnapshotCloneStatus struct {
	IsCloning          bool   `json:"isCloning"`
	Error              string `json:"error"`
	Progress           int    `json:"progress"`
	State              string `json:"state"`
	FromReplicaAddress string `json:"fromReplicaAddress"`
	SnapshotName       string `json:"snapshotName"`
}

// Should be the same values as in https://github.com/longhorn/longhorn-engine/blob/master/pkg/types/types.go
const (
	ProcessStateComplete   = "complete"
	ProcessStateError      = "error"
	ProcessStateInProgress = "in_progress"
)

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
	ShareManagerStateStopping = ShareManagerState("stopping")
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

// BackingImageDownloadState is replaced by BackingImageState.
type BackingImageDownloadState string

type BackingImageState string

const (
	BackingImageStatePending          = BackingImageState("pending")
	BackingImageStateStarting         = BackingImageState("starting")
	BackingImageStateReadyForTransfer = BackingImageState("ready-for-transfer")
	BackingImageStateReady            = BackingImageState("ready")
	BackingImageStateInProgress       = BackingImageState("in-progress")
	BackingImageStateFailed           = BackingImageState("failed")
	BackingImageStateUnknown          = BackingImageState("unknown")
)

type BackingImageSpec struct {
	Disks            map[string]struct{}        `json:"disks"`
	Checksum         string                     `json:"checksum"`
	SourceType       BackingImageDataSourceType `json:"sourceType"`
	SourceParameters map[string]string          `json:"sourceParameters"`

	// Deprecated: This kind of info will be included in the related BackingImageDataSource.
	ImageURL string `json:"imageURL"`
}

type BackingImageStatus struct {
	OwnerID           string                                 `json:"ownerID"`
	UUID              string                                 `json:"uuid"`
	Size              int64                                  `json:"size"`
	Checksum          string                                 `json:"checksum"`
	DiskFileStatusMap map[string]*BackingImageDiskFileStatus `json:"diskFileStatusMap"`
	DiskLastRefAtMap  map[string]string                      `json:"diskLastRefAtMap"`

	// Deprecated: Replaced by field `State` in `DiskFileStatusMap`.
	DiskDownloadStateMap map[string]BackingImageDownloadState `json:"diskDownloadStateMap"`
	// Deprecated: Replaced by field `Progress` in `DiskFileStatusMap`.
	DiskDownloadProgressMap map[string]int `json:"diskDownloadProgressMap"`
}

type BackingImageDiskFileStatus struct {
	State                   BackingImageState `json:"state"`
	Progress                int               `json:"progress"`
	Message                 string            `json:"message"`
	LastStateTransitionTime string            `json:"lastStateTransitionTime"`
}

type BackingImageManagerState string

const (
	BackingImageManagerStateError    = BackingImageManagerState("error")
	BackingImageManagerStateRunning  = BackingImageManagerState("running")
	BackingImageManagerStateStopped  = BackingImageManagerState("stopped")
	BackingImageManagerStateStarting = BackingImageManagerState("starting")
	BackingImageManagerStateUnknown  = BackingImageManagerState("unknown")
)

type BackingImageManagerSpec struct {
	Image         string            `json:"image"`
	NodeID        string            `json:"nodeID"`
	DiskUUID      string            `json:"diskUUID"`
	DiskPath      string            `json:"diskPath"`
	BackingImages map[string]string `json:"backingImages"`
}

type BackingImageManagerStatus struct {
	OwnerID             string                          `json:"ownerID"`
	CurrentState        BackingImageManagerState        `json:"currentState"`
	BackingImageFileMap map[string]BackingImageFileInfo `json:"backingImageFileMap"`
	IP                  string                          `json:"ip"`
	APIMinVersion       int                             `json:"apiMinVersion"`
	APIVersion          int                             `json:"apiVersion"`
}

type BackingImageFileInfo struct {
	Name                 string            `json:"name"`
	UUID                 string            `json:"uuid"`
	Size                 int64             `json:"size"`
	State                BackingImageState `json:"state"`
	CurrentChecksum      string            `json:"currentChecksum"`
	Message              string            `json:"message"`
	SendingReference     int               `json:"sendingReference"`
	SenderManagerAddress string            `json:"senderManagerAddress"`
	Progress             int               `json:"progress"`

	// Deprecated: This field is useless now. The manager of backing image files doesn't care if a file is downloaded and how.
	URL string `json:"url"`
	// Deprecated: This field is useless.
	Directory string `json:"directory"`
	// Deprecated: This field is renamed to `Progress`.
	DownloadProgress int `json:"downloadProgress"`
}

type BackingImageDataSourceType string

const (
	BackingImageDataSourceTypeDownload         = BackingImageDataSourceType("download")
	BackingImageDataSourceTypeUpload           = BackingImageDataSourceType("upload")
	BackingImageDataSourceTypeExportFromVolume = BackingImageDataSourceType("export-from-volume")
)

const (
	DataSourceTypeDownloadParameterURL                = "url"
	DataSourceTypeExportFromVolumeParameterVolumeName = "volume-name"
	DataSourceTypeExportFromVolumeParameterExportType = "export-type"

	DataSourceTypeExportFromVolumeParameterVolumeSize    = "volume-size"
	DataSourceTypeExportFromVolumeParameterSnapshotName  = "snapshot-name"
	DataSourceTypeExportFromVolumeParameterSenderAddress = "sender-address"

	DataSourceTypeExportFromVolumeParameterExportTypeRAW   = "raw"
	DataSourceTypeExportFromVolumeParameterExportTypeQCOW2 = "qcow2"
)

type BackingImageDataSourceSpec struct {
	NodeID          string                     `json:"nodeID"`
	DiskUUID        string                     `json:"diskUUID"`
	DiskPath        string                     `json:"diskPath"`
	Checksum        string                     `json:"checksum"`
	SourceType      BackingImageDataSourceType `json:"sourceType"`
	Parameters      map[string]string          `json:"parameters"`
	FileTransferred bool                       `json:"fileTransferred"`
}

type BackingImageDataSourceStatus struct {
	OwnerID           string            `json:"ownerID"`
	RunningParameters map[string]string `json:"runningParameters"`
	CurrentState      BackingImageState `json:"currentState"`
	Size              int64             `json:"size"`
	Progress          int               `json:"progress"`
	Checksum          string            `json:"checksum"`
	Message           string            `json:"message"`
}

type BackupTargetSpec struct {
	BackupTargetURL  string          `json:"backupTargetURL"`
	CredentialSecret string          `json:"credentialSecret"`
	PollInterval     metav1.Duration `json:"pollInterval"`
	SyncRequestedAt  time.Time       `json:"syncRequestedAt"`
}

const (
	BackupTargetConditionTypeUnavailable = "Unavailable"

	BackupTargetConditionReasonUnavailable = "Unavailable"
)

type BackupTargetStatus struct {
	OwnerID      string               `json:"ownerID"`
	Available    bool                 `json:"available"`
	Conditions   map[string]Condition `json:"conditions"`
	LastSyncedAt time.Time            `json:"lastSyncedAt"`
}

type BackupVolumeSpec struct {
	SyncRequestedAt time.Time `json:"syncRequestedAt"`
}

type BackupVolumeStatus struct {
	OwnerID              string            `json:"ownerID"`
	LastModificationTime time.Time         `json:"lastModificationTime"`
	Size                 string            `json:"size"`
	Labels               map[string]string `json:"labels"`
	CreatedAt            string            `json:"createdAt"`
	LastBackupName       string            `json:"lastBackupName"`
	LastBackupAt         string            `json:"lastBackupAt"`
	DataStored           string            `json:"dataStored"`
	Messages             map[string]string `json:"messages"`
	BackingImageName     string            `json:"backingImageName"`
	BackingImageChecksum string            `json:"backingImageChecksum"`
	LastSyncedAt         time.Time         `json:"lastSyncedAt"`
}

type SnapshotBackupSpec struct {
	SyncRequestedAt time.Time         `json:"syncRequestedAt"`
	SnapshotName    string            `json:"snapshotName"`
	Labels          map[string]string `json:"labels"`
}

type BackupState string

const (
	BackupStateInProgress = BackupState("InProgress")
	BackupStateCompleted  = BackupState("Completed")
	BackupStateError      = BackupState("Error")
	BackupStateUnknown    = BackupState("Unknown")
)

type SnapshotBackupStatus struct {
	OwnerID                string            `json:"ownerID"`
	State                  BackupState       `json:"state"`
	URL                    string            `json:"url"`
	SnapshotName           string            `json:"snapshotName"`
	SnapshotCreatedAt      string            `json:"snapshotCreatedAt"`
	BackupCreatedAt        string            `json:"backupCreatedAt"`
	Size                   string            `json:"size"`
	Labels                 map[string]string `json:"labels"`
	Messages               map[string]string `json:"messages"`
	VolumeName             string            `json:"volumeName"`
	VolumeSize             string            `json:"volumeSize"`
	VolumeCreated          string            `json:"volumeCreated"`
	VolumeBackingImageName string            `json:"volumeBackingImageName"`
	LastSyncedAt           time.Time         `json:"lastSyncedAt"`
}

type RecurringJobSpec struct {
	Name        string            `json:"name"`
	Groups      []string          `json:"groups,omitempty"`
	Task        RecurringJobType  `json:"task"`
	Cron        string            `json:"cron"`
	Retain      int               `json:"retain"`
	Concurrency int               `json:"concurrency"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type RecurringJobStatus struct {
	OwnerID string `json:"ownerID"`
}

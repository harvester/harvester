package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type ReplicaMode string

const (
	ReplicaModeRW  = ReplicaMode("RW")
	ReplicaModeWO  = ReplicaMode("WO")
	ReplicaModeERR = ReplicaMode("ERR")
)

type EngineBackupStatus struct {
	// +optional
	Progress int `json:"progress"`
	// +optional
	BackupURL string `json:"backupURL,omitempty"`
	// +optional
	Error string `json:"error,omitempty"`
	// +optional
	SnapshotName string `json:"snapshotName"`
	// +optional
	State string `json:"state"`
	// +optional
	ReplicaAddress string `json:"replicaAddress"`
}

type RestoreStatus struct {
	// +optional
	IsRestoring bool `json:"isRestoring"`
	// +optional
	LastRestored string `json:"lastRestored"`
	// +optional
	CurrentRestoringBackup string `json:"currentRestoringBackup"`
	// +optional
	Progress int `json:"progress,omitempty"`
	// +optional
	Error string `json:"error,omitempty"`
	// +optional
	Filename string `json:"filename,omitempty"`
	// +optional
	State string `json:"state"`
	// +optional
	BackupURL string `json:"backupURL"`
}

type PurgeStatus struct {
	// +optional
	Error string `json:"error"`
	// +optional
	IsPurging bool `json:"isPurging"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	State string `json:"state"`
}

type HashStatus struct {
	// +optional
	State string `json:"state"`
	// +optional
	Checksum string `json:"checksum"`
	// +optional
	Error string `json:"error"`
	// +optional
	SilentlyCorrupted bool `json:"silentlyCorrupted"`
}

type RebuildStatus struct {
	// +optional
	Error string `json:"error"`
	// +optional
	IsRebuilding bool `json:"isRebuilding"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	State string `json:"state"`
	// +optional
	FromReplicaAddress string `json:"fromReplicaAddress"`
}

type SnapshotCloneStatus struct {
	// +optional
	IsCloning bool `json:"isCloning"`
	// +optional
	Error string `json:"error"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	State string `json:"state"`
	// +optional
	FromReplicaAddress string `json:"fromReplicaAddress"`
	// +optional
	SnapshotName string `json:"snapshotName"`
}

type SnapshotInfo struct {
	// +optional
	Name string `json:"name"`
	// +optional
	Parent string `json:"parent"`
	// +optional
	// +nullable
	Children map[string]bool `json:"children"`
	// +optional
	Removed bool `json:"removed"`
	// +optional
	UserCreated bool `json:"usercreated"`
	// +optional
	Created string `json:"created"`
	// +optional
	Size string `json:"size"`
	// +optional
	// +nullable
	Labels map[string]string `json:"labels"`
}

// EngineSpec defines the desired state of the Longhorn engine
type EngineSpec struct {
	InstanceSpec `json:""`
	// +optional
	Frontend VolumeFrontend `json:"frontend"`
	// +optional
	ReplicaAddressMap map[string]string `json:"replicaAddressMap"`
	// +optional
	UpgradedReplicaAddressMap map[string]string `json:"upgradedReplicaAddressMap"`
	// +optional
	BackupVolume string `json:"backupVolume"`
	// +optional
	RequestedBackupRestore string `json:"requestedBackupRestore"`
	// +optional
	RequestedDataSource VolumeDataSource `json:"requestedDataSource"`
	// +optional
	DisableFrontend bool `json:"disableFrontend"`
	// +optional
	RevisionCounterDisabled bool `json:"revisionCounterDisabled"`
	// +optional
	UnmapMarkSnapChainRemovedEnabled bool `json:"unmapMarkSnapChainRemovedEnabled"`
	// +optional
	Active bool `json:"active"`
	// +optional
	SnapshotMaxCount int `json:"snapshotMaxCount"`
	// +kubebuilder:validation:Type=string
	// +optional
	SnapshotMaxSize int64 `json:"snapshotMaxSize,string"`
}

// EngineStatus defines the observed state of the Longhorn engine
type EngineStatus struct {
	InstanceStatus `json:""`
	// +kubebuilder:validation:Type=string
	// +optional
	CurrentSize int64 `json:"currentSize,string"`
	// +optional
	// +nullable
	CurrentReplicaAddressMap map[string]string `json:"currentReplicaAddressMap"`
	// +optional
	// +nullable
	ReplicaModeMap map[string]ReplicaMode `json:"replicaModeMap"`
	// +optional
	// ReplicaTransitionTimeMap records the time a replica in ReplicaModeMap transitions from one mode to another (or
	// from not being in the ReplicaModeMap to being in it). This information is sometimes required by other controllers
	// (e.g. the volume controller uses it to determine the correct value for replica.Spec.lastHealthyAt).
	ReplicaTransitionTimeMap map[string]string `json:"replicaTransitionTimeMap"`
	// +optional
	Endpoint string `json:"endpoint"`
	// +optional
	LastRestoredBackup string `json:"lastRestoredBackup"`
	// +optional
	// +nullable
	BackupStatus map[string]*EngineBackupStatus `json:"backupStatus"`
	// +optional
	// +nullable
	RestoreStatus map[string]*RestoreStatus `json:"restoreStatus"`
	// +optional
	// +nullable
	PurgeStatus map[string]*PurgeStatus `json:"purgeStatus"`
	// +optional
	// +nullable
	RebuildStatus map[string]*RebuildStatus `json:"rebuildStatus"`
	// +optional
	// +nullable
	CloneStatus map[string]*SnapshotCloneStatus `json:"cloneStatus"`
	// +optional
	// +nullable
	Snapshots map[string]*SnapshotInfo `json:"snapshots"`
	// +optional
	SnapshotsError string `json:"snapshotsError"`
	// +optional
	IsExpanding bool `json:"isExpanding"`
	// +optional
	LastExpansionError string `json:"lastExpansionError"`
	// +optional
	LastExpansionFailedAt string `json:"lastExpansionFailedAt"`
	// +optional
	UnmapMarkSnapChainRemovedEnabled bool `json:"unmapMarkSnapChainRemovedEnabled"`
	// +optional
	SnapshotMaxCount int `json:"snapshotMaxCount"`
	// +kubebuilder:validation:Type=string
	// +optional
	SnapshotMaxSize int64 `json:"snapshotMaxSize,string"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhe
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Data Engine",type=string,JSONPath=`.spec.dataEngine`,description="The data engine of the engine"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the engine"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node that the engine is on"
// +kubebuilder:printcolumn:name="InstanceManager",type=string,JSONPath=`.status.instanceManagerName`,description="The instance manager of the engine"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.currentImage`,description="The current image of the engine"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Engine is where Longhorn stores engine object.
type Engine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EngineSpec   `json:"spec,omitempty"`
	Status EngineStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EngineList is a list of Engines.
type EngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Engine `json:"items"`
}

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type ReplicaMode string

const (
	ReplicaModeRW  = ReplicaMode("RW")
	ReplicaModeWO  = ReplicaMode("WO")
	ReplicaModeERR = ReplicaMode("ERR")
)

type EngineBackupStatus struct {
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

type SnapshotInfo struct {
	Name        string            `json:"name"`
	Parent      string            `json:"parent"`
	Children    map[string]bool   `json:"children"`
	Removed     bool              `json:"removed"`
	UserCreated bool              `json:"usercreated"`
	Created     string            `json:"created"`
	Size        string            `json:"size"`
	Labels      map[string]string `json:"labels"`
}

// EngineSpec defines the desired state of the Longhorn engine
type EngineSpec struct {
	InstanceSpec              `json:""`
	Frontend                  VolumeFrontend    `json:"frontend"`
	ReplicaAddressMap         map[string]string `json:"replicaAddressMap"`
	UpgradedReplicaAddressMap map[string]string `json:"upgradedReplicaAddressMap"`
	BackupVolume              string            `json:"backupVolume"`
	RequestedBackupRestore    string            `json:"requestedBackupRestore"`
	RequestedDataSource       VolumeDataSource  `json:"requestedDataSource"`
	DisableFrontend           bool              `json:"disableFrontend"`
	RevisionCounterDisabled   bool              `json:"revisionCounterDisabled"`
	Active                    bool              `json:"active"`
}

// EngineStatus defines the observed state of the Longhorn engine
type EngineStatus struct {
	InstanceStatus           `json:""`
	CurrentSize              int64                           `json:"currentSize,string"`
	CurrentReplicaAddressMap map[string]string               `json:"currentReplicaAddressMap"`
	ReplicaModeMap           map[string]ReplicaMode          `json:"replicaModeMap"`
	Endpoint                 string                          `json:"endpoint"`
	LastRestoredBackup       string                          `json:"lastRestoredBackup"`
	BackupStatus             map[string]*EngineBackupStatus  `json:"backupStatus"`
	RestoreStatus            map[string]*RestoreStatus       `json:"restoreStatus"`
	PurgeStatus              map[string]*PurgeStatus         `json:"purgeStatus"`
	RebuildStatus            map[string]*RebuildStatus       `json:"rebuildStatus"`
	CloneStatus              map[string]*SnapshotCloneStatus `json:"cloneStatus"`
	Snapshots                map[string]*SnapshotInfo        `json:"snapshots"`
	SnapshotsError           string                          `json:"snapshotsError"`
	IsExpanding              bool                            `json:"isExpanding"`
	LastExpansionError       string                          `json:"lastExpansionError"`
	LastExpansionFailedAt    string                          `json:"lastExpansionFailedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhe
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the engine"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node that the engine is on"
// +kubebuilder:printcolumn:name="InstanceManager",type=string,JSONPath=`.status.instanceManagerName`,description="The instance manager of the engine"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.currentImage`,description="The current image of the engine"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Engine is where Longhorn stores engine object.
type Engine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec EngineSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status EngineStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EngineList is a list of Engines.
type EngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Engine `json:"items"`
}

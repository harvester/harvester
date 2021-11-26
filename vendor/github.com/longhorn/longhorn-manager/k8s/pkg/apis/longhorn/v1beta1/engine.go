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
	Active                    bool              `json:"active"`
}

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
	Snapshots                map[string]*Snapshot            `json:"snapshots"`
	SnapshotsError           string                          `json:"snapshotsError"`
	IsExpanding              bool                            `json:"isExpanding"`
	LastExpansionError       string                          `json:"lastExpansionError"`
	LastExpansionFailedAt    string                          `json:"lastExpansionFailedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Engine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              EngineSpec   `json:"spec"`
	Status            EngineStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Engine `json:"items"`
}

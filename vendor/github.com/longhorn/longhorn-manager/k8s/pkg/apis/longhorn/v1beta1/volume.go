package v1beta1

import (
	"fmt"

	"github.com/jinzhu/copier"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
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

type VolumeDataSourceType string

const (
	VolumeDataSourceTypeBackup   = VolumeDataSourceType("backup") // Planing to move FromBackup field into DataSource field
	VolumeDataSourceTypeSnapshot = VolumeDataSourceType("snapshot")
	VolumeDataSourceTypeVolume   = VolumeDataSourceType("volume")
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

// Deprecated: This field is useless.
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

// VolumeSpec defines the desired state of the Longhorn volume
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

// VolumeStatus defines the observed state of the Longhorn volume
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
// +kubebuilder:resource:shortName=lhv
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The state of the volume"
// +kubebuilder:printcolumn:name="Robustness",type=string,JSONPath=`.status.robustness`,description="The robustness of the volume"
// +kubebuilder:printcolumn:name="Scheduled",type=string,JSONPath=`.status.conditions['scheduled']['status']`,description="The scheduled condition of the volume"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`,description="The size of the volume"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.currentNodeID`,description="The node that the volume is currently attaching to"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Volume is where Longhorn stores volume object.
type Volume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec VolumeSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status VolumeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeList is a list of Volumes.
type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Volume `json:"items"`
}

// ConvertTo converts from spoke version (v1beta1) to hub version (v1beta2)
func (v *Volume) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta2.Volume:
		vV1beta2 := dst.(*v1beta2.Volume)
		vV1beta2.ObjectMeta = v.ObjectMeta
		if err := copier.Copy(&vV1beta2.Spec, &v.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&vV1beta2.Status, &v.Status); err != nil {
			return err
		}

		if v.Spec.DataLocality == "" {
			vV1beta2.Spec.DataLocality = v1beta2.DataLocality(DataLocalityDisabled)
		}

		// Copy status.conditions from map to slice
		dstConditions, err := copyConditionsFromMapToSlice(v.Status.Conditions)
		if err != nil {
			return err
		}
		vV1beta2.Status.Conditions = dstConditions

		// Copy status.KubernetesStatus
		if err := copier.Copy(&vV1beta2.Status.KubernetesStatus, &v.Status.KubernetesStatus); err != nil {
			return err
		}

		// Copy status.CloneStatus
		return copier.Copy(&vV1beta2.Status.CloneStatus, &v.Status.CloneStatus)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

// ConvertFrom converts from hub version (v1beta2) to spoke version (v1beta1)
func (v *Volume) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta2.Volume:
		vV1beta2 := src.(*v1beta2.Volume)
		v.ObjectMeta = vV1beta2.ObjectMeta
		if err := copier.Copy(&v.Spec, &vV1beta2.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&v.Status, &vV1beta2.Status); err != nil {
			return err
		}

		// Copy status.conditions from slice to map
		dstConditions, err := copyConditionFromSliceToMap(vV1beta2.Status.Conditions)
		if err != nil {
			return err
		}
		v.Status.Conditions = dstConditions

		// Copy status.KubernetesStatus
		if err := copier.Copy(&v.Status.KubernetesStatus, &vV1beta2.Status.KubernetesStatus); err != nil {
			return err
		}

		// Copy status.CloneStatus
		return copier.Copy(&v.Status.CloneStatus, &vV1beta2.Status.CloneStatus)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

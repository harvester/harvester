package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	DiskConditionTypeError       = "Error"
)

const (
	DiskConditionReasonDiskPressure          = "DiskPressure"
	DiskConditionReasonDiskFilesystemChanged = "DiskFilesystemChanged"
	DiskConditionReasonNoDiskInfo            = "NoDiskInfo"
	DiskConditionReasonDiskNotReady          = "DiskNotReady"
)

const (
	ErrorReplicaScheduleInsufficientStorage              = "insufficient storage"
	ErrorReplicaScheduleDiskNotFound                     = "disk not found"
	ErrorReplicaScheduleDiskUnavailable                  = "disks are unavailable"
	ErrorReplicaScheduleSchedulingSettingsRetrieveFailed = "failed to retrieve scheduling settings failed to retrieve"
	ErrorReplicaScheduleTagsNotFulfilled                 = "tags not fulfilled"
	ErrorReplicaScheduleNodeNotFound                     = "node not found"
	ErrorReplicaScheduleNodeUnavailable                  = "nodes are unavailable"
	ErrorReplicaScheduleEngineImageNotReady              = "none of the node candidates contains a ready engine image"
	ErrorReplicaScheduleHardNodeAffinityNotSatisfied     = "hard affinity cannot be satisfied"
	ErrorReplicaScheduleSchedulingFailed                 = "replica scheduling failed"
)

type DiskSpec struct {
	// +optional
	Path string `json:"path"`
	// +optional
	AllowScheduling bool `json:"allowScheduling"`
	// +optional
	EvictionRequested bool `json:"evictionRequested"`
	// +optional
	StorageReserved int64 `json:"storageReserved"`
	// +optional
	Tags []string `json:"tags"`
}

type DiskStatus struct {
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
	// +optional
	StorageAvailable int64 `json:"storageAvailable"`
	// +optional
	StorageScheduled int64 `json:"storageScheduled"`
	// +optional
	StorageMaximum int64 `json:"storageMaximum"`
	// +optional
	// +nullable
	ScheduledReplica map[string]int64 `json:"scheduledReplica"`
	// +optional
	DiskUUID string `json:"diskUUID"`
}

// NodeSpec defines the desired state of the Longhorn node
type NodeSpec struct {
	// +optional
	Name string `json:"name"`
	// +optional
	Disks map[string]DiskSpec `json:"disks"`
	// +optional
	AllowScheduling bool `json:"allowScheduling"`
	// +optional
	EvictionRequested bool `json:"evictionRequested"`
	// +optional
	Tags []string `json:"tags"`
	// +optional
	EngineManagerCPURequest int `json:"engineManagerCPURequest"`
	// +optional
	ReplicaManagerCPURequest int `json:"replicaManagerCPURequest"`
}

// NodeStatus defines the observed state of the Longhorn node
type NodeStatus struct {
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
	// +optional
	// +nullable
	DiskStatus map[string]*DiskStatus `json:"diskStatus"`
	// +optional
	Region string `json:"region"`
	// +optional
	Zone string `json:"zone"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhn
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`,description="Indicate whether the node is ready"
// +kubebuilder:printcolumn:name="AllowScheduling",type=boolean,JSONPath=`.spec.allowScheduling`,description="Indicate whether the user disabled/enabled replica scheduling for the node"
// +kubebuilder:printcolumn:name="Schedulable",type=string,JSONPath=`.status.conditions[?(@.type=='Schedulable')].status`,description="Indicate whether Longhorn can schedule replicas on the node"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Node is where Longhorn stores Longhorn node object.
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeList is a list of Nodes.
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

// Hub defines the current version (v1beta2) is the storage version
// so mark this as Hub
func (n *Node) Hub() {}

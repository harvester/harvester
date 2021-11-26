package v1beta1

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
)

const (
	DiskConditionReasonDiskPressure          = "DiskPressure"
	DiskConditionReasonDiskFilesystemChanged = "DiskFilesystemChanged"
	DiskConditionReasonNoDiskInfo            = "NoDiskInfo"
	DiskConditionReasonDiskNotReady          = "DiskNotReady"
)

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

type NodeSpec struct {
	Name                     string              `json:"name"`
	Disks                    map[string]DiskSpec `json:"disks"`
	AllowScheduling          bool                `json:"allowScheduling"`
	EvictionRequested        bool                `json:"evictionRequested"`
	Tags                     []string            `json:"tags"`
	EngineManagerCPURequest  int                 `json:"engineManagerCPURequest"`
	ReplicaManagerCPURequest int                 `json:"replicaManagerCPURequest"`
}

type NodeStatus struct {
	Conditions map[string]Condition   `json:"conditions"`
	DiskStatus map[string]*DiskStatus `json:"diskStatus"`
	Region     string                 `json:"region"`
	Zone       string                 `json:"zone"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NodeSpec   `json:"spec"`
	Status            NodeStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Node `json:"items"`
}

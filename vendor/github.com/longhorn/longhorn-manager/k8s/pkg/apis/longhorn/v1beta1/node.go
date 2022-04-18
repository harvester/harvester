package v1beta1

import (
	"fmt"

	"github.com/jinzhu/copier"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

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

// NodeSpec defines the desired state of the Longhorn node
type NodeSpec struct {
	Name                     string              `json:"name"`
	Disks                    map[string]DiskSpec `json:"disks"`
	AllowScheduling          bool                `json:"allowScheduling"`
	EvictionRequested        bool                `json:"evictionRequested"`
	Tags                     []string            `json:"tags"`
	EngineManagerCPURequest  int                 `json:"engineManagerCPURequest"`
	ReplicaManagerCPURequest int                 `json:"replicaManagerCPURequest"`
}

// NodeStatus defines the observed state of the Longhorn node
type NodeStatus struct {
	Conditions map[string]Condition   `json:"conditions"`
	DiskStatus map[string]*DiskStatus `json:"diskStatus"`
	Region     string                 `json:"region"`
	Zone       string                 `json:"zone"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhn
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions['Ready']['status']`,description="Indicate whether the node is ready"
// +kubebuilder:printcolumn:name="AllowScheduling",type=boolean,JSONPath=`.spec.allowScheduling`,description="Indicate whether the user disabled/enabled replica scheduling for the node"
// +kubebuilder:printcolumn:name="Schedulable",type=string,JSONPath=`.status.conditions['Schedulable']['status']`,description="Indicate whether Longhorn can schedule replicas on the node"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Node is where Longhorn stores Longhorn node object.
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec NodeSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status NodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeList is a list of Nodes.
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

// ConvertTo converts from spoke verion (v1beta1) to hub version (v1beta2)
func (n *Node) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta2.Node:
		nV1beta2 := dst.(*v1beta2.Node)
		nV1beta2.ObjectMeta = n.ObjectMeta
		if err := copier.Copy(&nV1beta2.Spec, &n.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&nV1beta2.Status, &n.Status); err != nil {
			return err
		}

		// Copy status.conditions from map to slice
		dstConditions, err := copyConditionsFromMapToSlice(n.Status.Conditions)
		if err != nil {
			return err
		}
		nV1beta2.Status.Conditions = dstConditions

		// Copy status.diskStatus.conditioions from map to slice
		dstDiskStatus := make(map[string]*v1beta2.DiskStatus)
		for name, from := range n.Status.DiskStatus {
			to := &v1beta2.DiskStatus{}
			if err := copier.Copy(to, from); err != nil {
				return err
			}
			conditions, err := copyConditionsFromMapToSlice(from.Conditions)
			if err != nil {
				return err
			}
			to.Conditions = conditions
			dstDiskStatus[name] = to
		}
		nV1beta2.Status.DiskStatus = dstDiskStatus
		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

// ConvertFrom converts from hub version (v1beta2) to spoke version (v1beta1)
func (n *Node) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta2.Node:
		nV1beta2 := src.(*v1beta2.Node)
		n.ObjectMeta = nV1beta2.ObjectMeta
		if err := copier.Copy(&n.Spec, &nV1beta2.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&n.Status, &nV1beta2.Status); err != nil {
			return err
		}

		// Copy status.conditions from slice to map
		dstConditions, err := copyConditionFromSliceToMap(nV1beta2.Status.Conditions)
		if err != nil {
			return err
		}
		n.Status.Conditions = dstConditions

		// Copy status.diskStatus.conditioions from slice to map
		dstDiskStatus := make(map[string]*DiskStatus)
		for name, from := range nV1beta2.Status.DiskStatus {
			to := &DiskStatus{}
			if err := copier.Copy(to, from); err != nil {
				return err
			}
			conditions, err := copyConditionFromSliceToMap(from.Conditions)
			if err != nil {
				return err
			}
			to.Conditions = conditions
			dstDiskStatus[name] = to
		}
		n.Status.DiskStatus = dstDiskStatus
		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

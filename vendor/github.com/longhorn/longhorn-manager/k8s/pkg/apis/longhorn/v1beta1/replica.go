package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Replica struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ReplicaSpec   `json:"spec"`
	Status            ReplicaStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ReplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Replica `json:"items"`
}

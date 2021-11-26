package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type EngineImageState string

const (
	EngineImageStateDeploying    = EngineImageState("deploying")
	EngineImageStateDeployed     = EngineImageState("deployed")
	EngineImageStateIncompatible = EngineImageState("incompatible")
	EngineImageStateError        = EngineImageState("error")
)

const (
	EngineImageConditionTypeReady = "ready"

	EngineImageConditionTypeReadyReasonDaemonSet = "daemonSet"
	EngineImageConditionTypeReadyReasonBinary    = "binary"
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

type EngineImageSpec struct {
	Image string `json:"image"`
}
type EngineImageStatus struct {
	OwnerID           string               `json:"ownerID"`
	State             EngineImageState     `json:"state"`
	RefCount          int                  `json:"refCount"`
	NoRefSince        string               `json:"noRefSince"`
	Conditions        map[string]Condition `json:"conditions"`
	NodeDeploymentMap map[string]bool      `json:"nodeDeploymentMap"`

	EngineVersionDetails
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EngineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              EngineImageSpec   `json:"spec"`
	Status            EngineImageStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EngineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []EngineImage `json:"items"`
}

package v1beta1

import (
	"fmt"

	"github.com/jinzhu/copier"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

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

// EngineImageSpec defines the desired state of the Longhorn engine image
type EngineImageSpec struct {
	Image string `json:"image"`
}

// EngineImageStatus defines the observed state of the Longhorn engine image
type EngineImageStatus struct {
	OwnerID           string               `json:"ownerID"`
	State             EngineImageState     `json:"state"`
	RefCount          int                  `json:"refCount"`
	NoRefSince        string               `json:"noRefSince"`
	Conditions        map[string]Condition `json:"conditions"`
	NodeDeploymentMap map[string]bool      `json:"nodeDeploymentMap"`

	EngineVersionDetails `json:""`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhei
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="State of the engine image"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`,description="The Longhorn engine image"
// +kubebuilder:printcolumn:name="RefCount",type=integer,JSONPath=`.status.refCount`,description="Number of resources using the engine image"
// +kubebuilder:printcolumn:name="BuildDate",type=date,JSONPath=`.status.buildDate`,description="The build date of the engine image"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EngineImage is where Longhorn stores engine image object.
type EngineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec EngineImageSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status EngineImageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EngineImageList is a list of EngineImages.
type EngineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EngineImage `json:"items"`
}

// ConvertTo converts from spoke version (v1beta1) to hub version (v1beta2)
func (ei *EngineImage) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta2.EngineImage:
		eiV1beta2 := dst.(*v1beta2.EngineImage)
		eiV1beta2.ObjectMeta = ei.ObjectMeta
		if err := copier.Copy(&eiV1beta2.Spec, &ei.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&eiV1beta2.Status, &ei.Status); err != nil {
			return err
		}

		// Copy status.conditions from map to slice
		dstConditions, err := copyConditionsFromMapToSlice(ei.Status.Conditions)
		if err != nil {
			return err
		}
		eiV1beta2.Status.Conditions = dstConditions

		// Copy status.EngineVersionDetails
		return copier.Copy(&eiV1beta2.Status.EngineVersionDetails, &ei.Status.EngineVersionDetails)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

// ConvertFrom converts from hub version (v1beta2) to spoke version (v1beta1)
func (ei *EngineImage) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta2.EngineImage:
		eiV1beta2 := src.(*v1beta2.EngineImage)
		ei.ObjectMeta = eiV1beta2.ObjectMeta
		if err := copier.Copy(&ei.Spec, &eiV1beta2.Spec); err != nil {
			return err
		}
		if err := copier.Copy(&ei.Status, &eiV1beta2.Status); err != nil {
			return err
		}

		// Copy status.conditions from slice to map
		dstConditions, err := copyConditionFromSliceToMap(eiV1beta2.Status.Conditions)
		if err != nil {
			return err
		}
		ei.Status.Conditions = dstConditions

		// Copy status.EngineVersionDetails
		return copier.Copy(&ei.Status.EngineVersionDetails, &eiV1beta2.Status.EngineVersionDetails)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

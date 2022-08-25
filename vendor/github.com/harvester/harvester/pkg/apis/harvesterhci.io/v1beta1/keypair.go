package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	KeyPairValidated condition.Cond = "validated"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=kp;kps,scope=Namespaced
// +kubebuilder:printcolumn:name="FINGER_PRINT",type=string,JSONPath=`.status.fingerPrint`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type KeyPair struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeyPairSpec   `json:"spec"`
	Status KeyPairStatus `json:"status,omitempty"`
}

type KeyPairSpec struct {
	// +kubebuilder:validation:Required
	PublicKey string `json:"publicKey"`
}

type KeyPairStatus struct {
	// +optional
	FingerPrint string `json:"fingerPrint,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type KeyGenInput struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

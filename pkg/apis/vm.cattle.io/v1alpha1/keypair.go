package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	KeyPairValidated condition.Cond = "validated"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KeyPair struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   KeyPairSpec   `json:"spec"`
	Status KeyPairStatus `json:"status"`
}

type KeyPairSpec struct {
	PublicKey string `json:"publicKey"`
}

type KeyPairStatus struct {
	FingerPrint string      `json:"fingerPrint"`
	Conditions  []Condition `json:"conditions"`
}

type KeyGenInput struct {
	Name string `json:"name"`
}

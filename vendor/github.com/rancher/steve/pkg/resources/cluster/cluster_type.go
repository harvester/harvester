package cluster

import (
	"github.com/rancher/wrangler/pkg/genericcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec"`
	Status            Status `json:"status"`
}

type Spec struct {
	DisplayName string `json:"displayName" norman:"required"`
	Internal    bool   `json:"internal,omitempty"`
	Description string `json:"description"`
}

type Status struct {
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Driver     string                              `json:"driver,omitempty"`
	Provider   string                              `json:"provider"`
	Version    *version.Info                       `json:"version,omitempty"`
}

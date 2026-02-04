package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AppliedConfigAnnotation struct {
	NTPServers string `json:"ntpServers,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=nconf,scope=Namespaced
// +kubebuilder:subresource:status

type NodeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeConfigSpec   `json:"spec"`
	Status            NodeConfigStatus `json:"status,omitempty"`
}

type NodeConfigSpec struct {
	NTPConfig      *NTPConfig      `json:"ntpConfigs,omitempty"`
	LonghornConfig *LonghornConfig `json:"longhornConfig,omitempty"`
}

type NTPConfig struct {
	NTPServers string `json:"ntpServers"`
}

type LonghornConfig struct {
	EnableV2DataEngine bool `json:"enableV2DataEngine,omitempty"`
}

type NodeConfigStatus struct {
	NTPConditions []ConfigStatus `json:"ntpConditions,omitempty"`
}

type ConfigStatus struct {
	Type   ConfigConditionType    `json:"type"`
	Status corev1.ConditionStatus `json:"status"`
	// +nullable
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// +nullable
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
}

type ConfigConditionType string

const (
	// first time when we create the config, is "WaitApplied"
	ConfigWaitApplied ConfigConditionType = "WaitApplied"

	// when the config is applied, is "Applied"
	ConfigApplied ConfigConditionType = "Applied"

	// when the applied config is modified, is "Modified"
	ConfigModified ConfigConditionType = "Modified"
)

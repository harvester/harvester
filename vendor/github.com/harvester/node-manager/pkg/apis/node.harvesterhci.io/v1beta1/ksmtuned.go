package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KsmdRun string

const (
	Stop  KsmdRun = "stop"  // set to 0 to stop ksmd from running but keep merged pages.
	Run   KsmdRun = "run"   // set to 1 to run ksmd.
	Prune KsmdRun = "prune" // set to 2 to stop ksmd and unmerge all pages currently merged, but leave mergeable areas registered for next run.
)

// KsmtunedMode defines the mode used by ksmtuned
type KsmtunedMode string

const (
	// StandardMode is default mode with new CR.
	StandardMode   KsmtunedMode = "standard"
	HighMode       KsmtunedMode = "high"
	CustomizedMode KsmtunedMode = "customized"
)

type KsmdPhase string

const (
	// KsmdStopped stop ksmd from running but keep merged pages,
	KsmdStopped KsmdPhase = "Stopped"
	KsmdRunning KsmdPhase = "Running"

	// KsmdPruned stop ksmd and unmerge all pages currently merged, but leave mergeable areas registered for next run.
	KsmdPruned KsmdPhase = "Pruned"

	KsmdUndefined KsmdPhase = "Undefined"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=ksmtd,scope=Cluster
// +kubebuilder:printcolumn:name="Run",type=string,JSONPath=`.spec.run`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:subresource:status

type Ksmtuned struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KsmtunedSpec   `json:"spec"`
	Status KsmtunedStatus `json:"status,omitempty"`
}

type KsmtunedSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=stop;run;prune
	// +kubebuilder:default=stop
	Run KsmdRun `json:"run"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=standard;high;customized
	// +kubebuilder:default=standard
	Mode KsmtunedMode `json:"mode"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=20
	ThresCoef uint `json:"thresCoef"`

	// +kubebuilder:validation:Maximum=1
	MergeAcrossNodes uint `json:"mergeAcrossNodes"`

	KsmtunedParameters KsmtunedParameters `json:"ksmtunedParameters"`
}

type KsmtunedStatus struct {
	// how many more sites are sharing them i.e. how much saved
	Sharing uint64 `json:"sharing"`

	// how many shared pages are being used
	Shared uint64 `json:"shared"`

	// how many pages unique but repeatedly checked for merging
	Unshared uint64 `json:"unshared"`

	// how many pages changing too fast to be placed in a tree
	Volatile uint64 `json:"volatile"`

	// how many times all mergeable areas have been scanned
	FullScans uint64 `json:"fullScans"`

	// the number of KSM pages that hit the max_page_sharing limit
	StableNodeChains uint64 `json:"stableNodeChains"`

	// number of duplicated KSM pages
	StableNodeDups uint64 `json:"stableNodeDups"`

	// ksmd status
	// +kubebuilder:validation:Enum=Stopped;Running;Pruned
	// +kubebuilder:default=Stopped
	KsmdPhase KsmdPhase `json:"ksmdPhase"`
}

type KsmtunedParameters struct {
	SleepMsec uint64 `json:"sleepMsec"`
	Boost     uint   `json:"boost"`
	Decay     uint   `json:"decay"`
	MinPages  uint   `json:"minPages"`
	MaxPages  uint   `json:"maxPages"`
}

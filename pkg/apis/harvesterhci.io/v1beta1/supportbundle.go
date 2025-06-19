package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	SupportBundleInitialized condition.Cond = "Initialized"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=sb;sbs,scope=Namespaced
// +kubebuilder:printcolumn:name="ISSUE_URL",type=string,JSONPath=`.spec.issueURL`
// +kubebuilder:printcolumn:name="DESCRIPTION",type="string",JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type SupportBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupportBundleSpec   `json:"spec"`
	Status SupportBundleStatus `json:"status,omitempty"`
}

type SupportBundleSpec struct {
	// +optional
	IssueURL string `json:"issueURL"`

	// +kubebuilder:validation:Required
	Description string `json:"description"`

	// +optional
	ExtraCollectionNamespaces []string `json:"extraCollectionNamespaces"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	// Number of minutes Harvester allows for the completion of the support bundle generation process.
	// Zero means no timeout.
	Timeout *int `json:"timeout,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	// Number of minutes Harvester waits before deleting a support bundle that has been packaged but not downloaded (either deliberately or unsuccessfully) or retained.
	Expiration int `json:"expiration,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	// Number of minutes Harvester allows for collection of logs and configurations (Harvester) on the nodes for the support bundle.
	NodeTimeout int `json:"nodeTimeout,omitempty"`
}

type SupportBundleStatus struct {
	// +optional
	State string `json:"state,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`

	// +optional
	Filename string `json:"filename,omitempty"`

	// +optional
	Filesize int64 `json:"filesize,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

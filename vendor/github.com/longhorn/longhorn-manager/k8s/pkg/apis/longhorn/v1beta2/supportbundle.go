package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SupportBundleState string

const (
	SupportBundleConditionTypeError   = "Error"
	SupportBundleConditionTypeManager = "Manager"

	SupportBundleMsgCreateManagerFmt = "Created deployment %v"
	SupportBundleMsgStartManagerFmt  = "Started support bundle manager for %v"
	SupportBundleMsgGeneratedFmt     = "Generated support bundle %v in %v"
	SupportBundleMsgInitFmt          = "Initialized %v"

	SupportBundleStatePurging    = SupportBundleState("Purging")
	SupportBundleStateDeleting   = SupportBundleState("Deleting")
	SupportBundleStateError      = SupportBundleState("Error")
	SupportBundleStateGenerating = SupportBundleState("Generating")
	SupportBundleStateNone       = SupportBundleState("")
	SupportBundleStateReady      = SupportBundleState("ReadyForDownload")
	SupportBundleStateReplaced   = SupportBundleState("Replaced")
	SupportBundleStateStarted    = SupportBundleState("Started")
	SupportBundleStateUnknown    = SupportBundleState("Unknown")
)

// SupportBundleSpec defines the desired state of the Longhorn SupportBundle
type SupportBundleSpec struct {
	// The preferred responsible controller node ID.
	// +optional
	NodeID string `json:"nodeID"`

	// The issue URL
	// +optional
	// +nullable
	IssueURL string `json:"issueURL"`
	// A brief description of the issue
	Description string `json:"description"`
}

// SupportBundleStatus defines the observed state of the Longhorn SupportBundle
type SupportBundleStatus struct {
	// The current responsible controller node ID
	// +optional
	OwnerID string `json:"ownerID"`

	// The support bundle manager image
	// +optional
	Image string `json:"image"`
	// The support bundle manager IP
	// +optional
	IP string `json:"managerIP"`

	// +optional
	State SupportBundleState `json:"state,omitempty"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	Filename string `json:"filename,omitempty"`
	// +optional
	Filesize int64 `json:"filesize,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbundle
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The state of the support bundle"
// +kubebuilder:printcolumn:name="Issue",type=string,JSONPath=`.spec.issueURL`,description="The issue URL"
// +kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`,description="A brief description of the issue"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SupportBundle is where Longhorn stores support bundle object
type SupportBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupportBundleSpec   `json:"spec,omitempty"`
	Status SupportBundleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SupportBundleList is a list of SupportBundles
type SupportBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupportBundle `json:"items"`
}

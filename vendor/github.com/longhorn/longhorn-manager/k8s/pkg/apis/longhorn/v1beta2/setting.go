package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhs
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Value",type=string,JSONPath=`.value`,description="The value of the setting"
// +kubebuilder:printcolumn:name="Applied",type=boolean,JSONPath=`.status.applied`,description="The setting is applied"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Setting is where Longhorn stores setting object.
type Setting struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The value of the setting.
	// - It can be a non-JSON formatted string that is applied to all the applicable data engines listed in the setting definition.
	// - It can be a JSON formatted string that contains values for applicable data engines listed in the setting definition's Default.
	Value string `json:"value"`

	// The status of the setting.
	Status SettingStatus `json:"status,omitempty"`
}

// SettingStatus defines the observed state of the Longhorn setting
type SettingStatus struct {
	// The setting is applied.
	Applied bool `json:"applied"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SettingList is a list of Settings.
type SettingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Setting `json:"items"`
}

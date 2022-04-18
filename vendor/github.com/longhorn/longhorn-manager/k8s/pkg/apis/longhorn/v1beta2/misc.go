package v1beta2

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

type Condition struct {
	// Type is the type of the condition.
	// +optional
	Type string `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	// +optional
	Status ConditionStatus `json:"status"`
	// Last time we probed the condition.
	// +optional
	LastProbeTime string `json:"lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime string `json:"lastTransitionTime"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message"`
}

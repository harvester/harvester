package v1beta1

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

type Condition struct {
	// Type is the type of the condition.
	Type string `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status ConditionStatus `json:"status"`
	// Last time we probed the condition.
	LastProbeTime string `json:"lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	Reason string `json:"reason"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message"`
}

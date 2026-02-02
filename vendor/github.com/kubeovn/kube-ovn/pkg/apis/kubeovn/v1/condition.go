package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants for condition
const (
	// Ready => controller considers this resource Ready
	Ready = "Ready"
	// Validated => Spec passed validating
	Validated = "Validated"
	// Init => controller is initializing this resource
	Init = "Init"
	// Error => last recorded error
	Error = "Error"

	ReasonInit = "Init"
)

// ConditionType encodes information on the condition
type ConditionType string

// Condition describes the state of an object at a certain point.
// +k8s:deepcopy-gen=true
type Condition struct {
	// Type of condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
	// ObservedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9,
	// the condition is out of date with respect to the current state of the instance.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Last time the condition was probed
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

type Conditions []Condition

func (c *Conditions) addCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string, generation int64) {
	if *c == nil {
		*c = make(Conditions, 0, 1)
	}
	now := metav1.Now()
	*c = append(*c, Condition{
		Type:               ctype,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// SetCondition creates or updates a condition
func (c *Conditions) SetCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string, generation int64) {
	var v *Condition
	for i := range *c {
		if (*c)[i].Type == ctype {
			v = &(*c)[i]
		}
	}
	if v == nil {
		c.addCondition(ctype, status, reason, message, generation)
		return
	}

	// check message ?
	if v.Status == status && v.Reason == reason && v.Message == message && v.ObservedGeneration == generation {
		return
	}

	now := metav1.Now()
	v.LastUpdateTime = now
	if v.Status != status {
		v.LastTransitionTime = now
	}
	v.Status = status
	v.Reason = reason
	v.Message = message
	v.ObservedGeneration = generation
}

// RemoveCondition removes the condition with the provided type.
func (c *Conditions) RemoveCondition(ctype ConditionType) {
	for i, v := range *c {
		if v.Type == ctype {
			(*c)[i] = (*c)[len(*c)-1]
			*c = (*c)[:len(*c)-1]
			break
		}
	}
}

// GetCondition get existing condition
func (c *Conditions) GetCondition(ctype ConditionType) *Condition {
	for _, v := range *c {
		if v.Type == ctype {
			return v.DeepCopy()
		}
	}
	return nil
}

// IsConditionTrue - if condition is true
func (c *Conditions) IsConditionTrue(ctype ConditionType, generation int64) bool {
	v := c.GetCondition(ctype)
	return v != nil && v.ObservedGeneration >= generation && v.Status == corev1.ConditionTrue
}

func (c *Conditions) SetReady(reason string, generation int64) {
	c.SetCondition(Ready, corev1.ConditionTrue, reason, "", generation)
}

// IsReady returns true if ready condition is set
func (c *Conditions) IsReady(generation int64) bool {
	return c.IsConditionTrue(Ready, generation)
}

func (c *Conditions) SetValidated(generation int64) {
	c.SetCondition(Validated, corev1.ConditionTrue, "", "", generation)
}

// IsValidated returns true if ready condition is set
func (c *Conditions) IsValidated(generation int64) bool {
	return c.IsConditionTrue(Validated, generation)
}

// ConditionReason - return condition reason
func (c *Conditions) ConditionReason(ctype ConditionType) string {
	if v := c.GetCondition(ctype); v != nil {
		return v.Reason
	}
	return ""
}

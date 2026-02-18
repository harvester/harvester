package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Vlan `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
type Vlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VlanSpec   `json:"spec"`
	Status VlanStatus `json:"status"`
}

type VlanSpec struct {
	// deprecated fields, use ID & Provider instead
	VlanID                int    `json:"vlanId,omitempty"`
	ProviderInterfaceName string `json:"providerInterfaceName,omitempty"`

	ID       int    `json:"id"`
	Provider string `json:"provider,omitempty"`
}

type VlanStatus struct {
	// +optional
	// +patchStrategy=merge
	Subnets []string `json:"subnets,omitempty"`

	Conflict bool `json:"conflict,omitempty"`

	// Conditions represents the latest state of the object
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

// SetVlanError - shortcut to set error condition
func (v *VlanStatus) SetVlanError(reason, message string) {
	v.SetVlanCondition(Error, reason, message)
}

// SetVlanCondition updates or creates a new condition
func (v *VlanStatus) SetVlanCondition(ctype ConditionType, reason, message string) {
	v.setVlanConditionValue(ctype, corev1.ConditionTrue, reason, message)
}

func (v *VlanStatus) setVlanConditionValue(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	var c *Condition
	for i := range v.Conditions {
		if v.Conditions[i].Type == ctype {
			c = &v.Conditions[i]
		}
	}
	if c == nil {
		v.addVlanCondition(ctype, status, reason, message)
	} else {
		// check message ?
		if c.Status == status && c.Reason == reason && c.Message == message {
			return
		}
		now := metav1.Now()
		c.LastUpdateTime = now
		if c.Status != status {
			c.LastTransitionTime = now
		}
		c.Status = status
		c.Reason = reason
		c.Message = message
	}
}

func (v *VlanStatus) addVlanCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	c := &Condition{
		Type:               ctype,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
	v.Conditions = append(v.Conditions, *c)
}

package v1

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ProviderNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProviderNetwork `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=provider-networks
type ProviderNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ProviderNetworkSpec   `json:"spec"`
	Status ProviderNetworkStatus `json:"status"`
}

type CustomInterface struct {
	Interface string   `json:"interface"`
	Nodes     []string `json:"nodes"`
}
type ProviderNetworkSpec struct {
	DefaultInterface string            `json:"defaultInterface,omitempty"`
	CustomInterfaces []CustomInterface `json:"customInterfaces,omitempty"`
	ExcludeNodes     []string          `json:"excludeNodes,omitempty"`
	ExchangeLinkName bool              `json:"exchangeLinkName,omitempty"`
}

type ProviderNetworkCondition struct {
	// Node name
	Node string `json:"node"`
	Condition
}

type ProviderNetworkStatus struct {
	// +optional
	Ready bool `json:"ready"`

	// +optional
	ReadyNodes []string `json:"readyNodes,omitempty"`

	// +optional
	NotReadyNodes []string `json:"notReadyNodes,omitempty"`

	// +optional
	Vlans []string `json:"vlans,omitempty"`

	// Conditions represents the latest state of the object
	// +optional
	Conditions []ProviderNetworkCondition `json:"conditions,omitempty"`
}

func (s *ProviderNetworkStatus) addNodeCondition(node string, ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	c := &ProviderNetworkCondition{
		Node: node,
		Condition: Condition{
			Type:               ctype,
			LastUpdateTime:     now,
			LastTransitionTime: now,
			Status:             status,
			Reason:             reason,
			Message:            message,
		},
	}
	s.Conditions = append(s.Conditions, *c)
}

// setConditionValue updates or creates a new condition
func (s *ProviderNetworkStatus) setNodeConditionValue(node string, ctype ConditionType, status corev1.ConditionStatus, reason, message string) bool {
	var c *ProviderNetworkCondition
	for i := range s.Conditions {
		if s.Conditions[i].Node == node && s.Conditions[i].Type == ctype {
			c = &s.Conditions[i]
		}
	}
	if c == nil {
		s.addNodeCondition(node, ctype, status, reason, message)
	} else {
		// check message ?
		if c.Status == status && c.Reason == reason && c.Message == message {
			return false
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

	return true
}

// RemoveNodeCondition removes the condition with the provided type.
func (s *ProviderNetworkStatus) RemoveNodeCondition(node string, ctype ConditionType) {
	for i, c := range s.Conditions {
		if c.Node == node && c.Type == ctype {
			s.Conditions[i] = s.Conditions[len(s.Conditions)-1]
			s.Conditions = s.Conditions[:len(s.Conditions)-1]
			break
		}
	}
}

// GetNodeCondition get existing condition
func (s *ProviderNetworkStatus) GetNodeCondition(node string, ctype ConditionType) *ProviderNetworkCondition {
	for i := range s.Conditions {
		if s.Conditions[i].Node == node && s.Conditions[i].Type == ctype {
			return &s.Conditions[i]
		}
	}
	return nil
}

// IsNodeConditionTrue - if condition is true
func (s *ProviderNetworkStatus) IsNodeConditionTrue(node string, ctype ConditionType) bool {
	if c := s.GetNodeCondition(node, ctype); c != nil {
		return c.Status == corev1.ConditionTrue
	}
	return false
}

// NodeIsReady returns true if ready condition is set
func (s *ProviderNetworkStatus) NodeIsReady(node string) bool {
	for _, c := range s.Conditions {
		if c.Node == node && c.Type == Ready && c.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// IsReady returns true if ready condition is set
func (s *ProviderNetworkStatus) IsReady() bool {
	for _, c := range s.Conditions {
		if c.Type == Ready && c.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// ConditionReason - return condition reason
func (s *ProviderNetworkStatus) ConditionReason(node string, ctype ConditionType) string {
	if c := s.GetNodeCondition(node, ctype); c != nil {
		return c.Reason
	}
	return ""
}

// SetNodeReady - shortcut to set ready condition to true
func (s *ProviderNetworkStatus) SetNodeReady(node, reason, message string) bool {
	return s.SetNodeCondition(node, Ready, reason, message)
}

// SetNodeNotReady - shortcut to set ready condition to false
func (s *ProviderNetworkStatus) SetNodeNotReady(node, reason, message string) bool {
	return s.ClearNodeCondition(node, Ready, reason, message)
}

// EnsureNodeCondition useful for adding default conditions
func (s *ProviderNetworkStatus) EnsureNodeCondition(node string, ctype ConditionType) bool {
	if c := s.GetNodeCondition(node, ctype); c != nil {
		return false
	}
	s.addNodeCondition(node, ctype, corev1.ConditionUnknown, ReasonInit, "Not Observed")
	return true
}

// EnsureNodeStandardConditions - helper to inject standard conditions
func (s *ProviderNetworkStatus) EnsureNodeStandardConditions(node string) bool {
	return s.EnsureNodeCondition(node, Ready)
}

// ClearNodeCondition updates or creates a new condition
func (s *ProviderNetworkStatus) ClearNodeCondition(node string, ctype ConditionType, reason, message string) bool {
	return s.setNodeConditionValue(node, ctype, corev1.ConditionFalse, reason, message)
}

// SetNodeCondition updates or creates a new condition
func (s *ProviderNetworkStatus) SetNodeCondition(node string, ctype ConditionType, reason, message string) bool {
	return s.setNodeConditionValue(node, ctype, corev1.ConditionTrue, reason, message)
}

// RemoveNodeConditions updates or creates a new condition
func (s *ProviderNetworkStatus) RemoveNodeConditions(node string) bool {
	var changed bool
	for i := 0; i < len(s.Conditions); {
		if s.Conditions[i].Node == node {
			changed = true
			s.Conditions = slices.Delete(s.Conditions, i, i+1)
		} else {
			i++
		}
	}
	return changed
}

package v1

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	ProtocolIPv4 = "IPv4"
	ProtocolIPv6 = "IPv6"
	ProtocolDual = "Dual"
)

const (
	GWDistributedType = "distributed"
	GWCentralizedType = "centralized"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Subnet `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   SubnetSpec   `json:"spec"`
	Status SubnetStatus `json:"status"`
}

type SubnetSpec struct {
	Default    bool     `json:"default"`
	Vpc        string   `json:"vpc,omitempty"`
	Protocol   string   `json:"protocol,omitempty"`
	Namespaces []string `json:"namespaces,omitempty"`
	CIDRBlock  string   `json:"cidrBlock"`
	Gateway    string   `json:"gateway"`
	ExcludeIps []string `json:"excludeIps,omitempty"`
	Provider   string   `json:"provider,omitempty"`

	GatewayType string `json:"gatewayType,omitempty"`
	GatewayNode string `json:"gatewayNode"`
	NatOutgoing bool   `json:"natOutgoing"`

	ExternalEgressGateway string `json:"externalEgressGateway,omitempty"`
	PolicyRoutingPriority uint32 `json:"policyRoutingPriority,omitempty"`
	PolicyRoutingTableID  uint32 `json:"policyRoutingTableID,omitempty"`
	Mtu                   uint32 `json:"mtu,omitempty"`

	Private      bool     `json:"private"`
	AllowSubnets []string `json:"allowSubnets,omitempty"`

	Vlan string   `json:"vlan,omitempty"`
	Vips []string `json:"vips,omitempty"`

	LogicalGateway         bool `json:"logicalGateway,omitempty"`
	DisableGatewayCheck    bool `json:"disableGatewayCheck,omitempty"`
	DisableInterConnection bool `json:"disableInterConnection,omitempty"`

	EnableDHCP    bool   `json:"enableDHCP,omitempty"`
	DHCPv4Options string `json:"dhcpV4Options,omitempty"`
	DHCPv6Options string `json:"dhcpV6Options,omitempty"`

	EnableIPv6RA  bool   `json:"enableIPv6RA,omitempty"`
	IPv6RAConfigs string `json:"ipv6RAConfigs,omitempty"`

	Acls           []ACL `json:"acls,omitempty"`
	AllowEWTraffic bool  `json:"allowEWTraffic,omitempty"`

	NatOutgoingPolicyRules []NatOutgoingPolicyRule `json:"natOutgoingPolicyRules,omitempty"`

	U2OInterconnectionIP    string `json:"u2oInterconnectionIP,omitempty"`
	U2OInterconnection      bool   `json:"u2oInterconnection,omitempty"`
	EnableLb                *bool  `json:"enableLb,omitempty"`
	EnableEcmp              bool   `json:"enableEcmp,omitempty"`
	EnableMulticastSnoop    bool   `json:"enableMulticastSnoop,omitempty"`
	EnableExternalLBAddress bool   `json:"enableExternalLBAddress,omitempty"`

	RouteTable         string                 `json:"routeTable,omitempty"`
	NamespaceSelectors []metav1.LabelSelector `json:"namespaceSelectors,omitempty"`
}

type ACL struct {
	Direction string `json:"direction,omitempty"`
	Priority  int    `json:"priority,omitempty"`
	Match     string `json:"match,omitempty"`
	Action    string `json:"action,omitempty"`
}

type NatOutgoingPolicyRule struct {
	Match  NatOutGoingPolicyMatch `json:"match"`
	Action string                 `json:"action"`
}

type NatOutGoingPolicyMatch struct {
	SrcIPs string `json:"srcIPs,omitempty"`
	DstIPs string `json:"dstIPs,omitempty"`
}

type NatOutgoingPolicyRuleStatus struct {
	RuleID string `json:"ruleID"`
	NatOutgoingPolicyRule
}
type SubnetStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	V4AvailableIPs         float64                       `json:"v4availableIPs"`
	V4AvailableIPRange     string                        `json:"v4availableIPrange"`
	V4UsingIPs             float64                       `json:"v4usingIPs"`
	V4UsingIPRange         string                        `json:"v4usingIPrange"`
	V6AvailableIPs         float64                       `json:"v6availableIPs"`
	V6AvailableIPRange     string                        `json:"v6availableIPrange"`
	V6UsingIPs             float64                       `json:"v6usingIPs"`
	V6UsingIPRange         string                        `json:"v6usingIPrange"`
	ActivateGateway        string                        `json:"activateGateway"`
	DHCPv4OptionsUUID      string                        `json:"dhcpV4OptionsUUID"`
	DHCPv6OptionsUUID      string                        `json:"dhcpV6OptionsUUID"`
	U2OInterconnectionIP   string                        `json:"u2oInterconnectionIP"`
	U2OInterconnectionMAC  string                        `json:"u2oInterconnectionMAC"`
	U2OInterconnectionVPC  string                        `json:"u2oInterconnectionVPC"`
	NatOutgoingPolicyRules []NatOutgoingPolicyRuleStatus `json:"natOutgoingPolicyRules"`
	McastQuerierIP         string                        `json:"mcastQuerierIP"`
	McastQuerierMAC        string                        `json:"mcastQuerierMAC"`
}

func (s *SubnetStatus) addCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	c := &Condition{
		Type:               ctype,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
	s.Conditions = append(s.Conditions, *c)
}

// setConditionValue updates or creates a new condition
func (s *SubnetStatus) setConditionValue(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	var c *Condition
	for i := range s.Conditions {
		if s.Conditions[i].Type == ctype {
			c = &s.Conditions[i]
		}
	}
	if c == nil {
		s.addCondition(ctype, status, reason, message)
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

// RemoveCondition removes the condition with the provided type.
func (s *SubnetStatus) RemoveCondition(ctype ConditionType) {
	for i, c := range s.Conditions {
		if c.Type == ctype {
			s.Conditions[i] = s.Conditions[len(s.Conditions)-1]
			s.Conditions = s.Conditions[:len(s.Conditions)-1]
			break
		}
	}
}

// GetCondition get existing condition
func (s *SubnetStatus) GetCondition(ctype ConditionType) *Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == ctype {
			return &s.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue - if condition is true
func (s *SubnetStatus) IsConditionTrue(ctype ConditionType) bool {
	if c := s.GetCondition(ctype); c != nil {
		return c.Status == corev1.ConditionTrue
	}
	return false
}

// IsReady returns true if ready condition is set
func (s *SubnetStatus) IsReady() bool { return s.IsConditionTrue(Ready) }

// IsNotReady returns true if ready condition is set
func (s *SubnetStatus) IsNotReady() bool { return !s.IsConditionTrue(Ready) }

// IsValidated returns true if ready condition is set
func (s *SubnetStatus) IsValidated() bool { return s.IsConditionTrue(Validated) }

// IsNotValidated returns true if ready condition is set
func (s *SubnetStatus) IsNotValidated() bool { return !s.IsConditionTrue(Validated) }

// ConditionReason - return condition reason
func (s *SubnetStatus) ConditionReason(ctype ConditionType) string {
	if c := s.GetCondition(ctype); c != nil {
		return c.Reason
	}
	return ""
}

// Ready - shortcut to set ready condition to true
func (s *SubnetStatus) Ready(reason, message string) {
	s.SetCondition(Ready, reason, message)
}

// NotReady - shortcut to set ready condition to false
func (s *SubnetStatus) NotReady(reason, message string) {
	s.ClearCondition(Ready, reason, message)
}

// Validated - shortcut to set validated condition to true
func (s *SubnetStatus) Validated(reason, message string) {
	s.SetCondition(Validated, reason, message)
}

// NotValidated - shortcut to set validated condition to false
func (s *SubnetStatus) NotValidated(reason, message string) {
	s.ClearCondition(Validated, reason, message)
}

// SetError - shortcut to set error condition
func (s *SubnetStatus) SetError(reason, message string) {
	s.SetCondition(Error, reason, message)
}

// ClearError - shortcut to set error condition
func (s *SubnetStatus) ClearError() {
	s.ClearCondition(Error, "NoError", "No error seen")
}

// EnsureCondition useful for adding default conditions
func (s *SubnetStatus) EnsureCondition(ctype ConditionType) {
	if c := s.GetCondition(ctype); c != nil {
		return
	}
	s.addCondition(ctype, corev1.ConditionUnknown, ReasonInit, "Not Observed")
}

// EnsureStandardConditions - helper to inject standard conditions
func (s *SubnetStatus) EnsureStandardConditions() {
	s.EnsureCondition(Ready)
	s.EnsureCondition(Validated)
	s.EnsureCondition(Error)
}

// ClearCondition updates or creates a new condition
func (s *SubnetStatus) ClearCondition(ctype ConditionType, reason, message string) {
	s.setConditionValue(ctype, corev1.ConditionFalse, reason, message)
}

// SetCondition updates or creates a new condition
func (s *SubnetStatus) SetCondition(ctype ConditionType, reason, message string) {
	s.setConditionValue(ctype, corev1.ConditionTrue, reason, message)
}

// RemoveAllConditions updates or creates a new condition
func (s *SubnetStatus) RemoveAllConditions() {
	s.Conditions = []Condition{}
}

// ClearAllConditions updates or creates a new condition
func (s *SubnetStatus) ClearAllConditions() {
	for i := range s.Conditions {
		s.Conditions[i].Status = corev1.ConditionFalse
	}
}

func (s *SubnetStatus) Bytes() ([]byte, error) {
	// {"availableIPs":65527,"usingIPs":9} => {"status": {"availableIPs":65527,"usingIPs":9}}
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}

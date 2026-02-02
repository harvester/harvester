package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TrafficPolicyLocal   = "Local"
	TrafficPolicyCluster = "Cluster"
)

// Phase represents resource phase
type Phase string

const (
	// PhasePending means the resource is pending and not processed yet
	PhasePending Phase = "Pending"
	// PhaseProcessing means the resource is being processed
	PhaseProcessing Phase = "Processing"
	// PhaseCompleted means the resource has been processed successfully
	PhaseCompleted Phase = "Completed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VpcEgressGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VpcEgressGateway `json:"items"`
}

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resourceName=vpc-egress-gateways
// vpc egress gateway is used to forward the egress traffic from the VPC to the external network
type VpcEgressGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VpcEgressGatewaySpec   `json:"spec"`
	Status VpcEgressGatewayStatus `json:"status"`
}

// VPC returns the VPC name
// If the VpcEgressGateway has no VPC specified in the spec, it will return the default VPC name
func (g *VpcEgressGateway) VPC(defaultVPC string) string {
	if g.Spec.VPC != "" {
		return g.Spec.VPC
	}
	return defaultVPC
}

// Ready returns true if the VpcEgressGateway has been processed successfully and is ready to serve traffic
func (g *VpcEgressGateway) Ready() bool {
	return g.Status.Ready && g.Status.Conditions.IsReady(g.Generation)
}

type VpcEgressGatewaySpec struct {
	// optional VPC name
	// if not specified, the default VPC will be used
	VPC string `json:"vpc,omitempty"`
	// workload replicas
	Replicas int32 `json:"replicas,omitempty"`
	// optional name prefix used to generate the workload
	// the workload name will be generated as <prefix><vpc-egress-gateway-name>
	Prefix string `json:"prefix,omitempty"`
	// optional image used by the workload
	// if not specified, the default image passed in by kube-ovn-controller will be used
	Image string `json:"image,omitempty"`
	// optional internal subnet used to create the workload
	// if not specified, the workload will be created in the default subnet of the VPC
	InternalSubnet string `json:"internalSubnet,omitempty"`
	// external subnet used to create the workload
	ExternalSubnet string `json:"externalSubnet"`
	// optional internal/external IPs used to create the workload
	// these IPs must be in the internal/external subnet
	// the IPs count must NOT be less than the replicas count
	InternalIPs []string `json:"internalIPs,omitempty"`
	ExternalIPs []string `json:"externalIPs,omitempty"`
	// namespace/pod selectors
	Selectors []VpcEgressGatewaySelector `json:"selectors,omitempty"`
	// optional traffic policy used to control the traffic routing
	// if not specified, the default traffic policy "Cluster" will be used
	// if set to "Local", traffic will be routed to the gateway pod/instance on the same node when available
	// currently it works only for the default vpc
	TrafficPolicy string `json:"trafficPolicy,omitempty"`

	// BFD configuration
	BFD VpcEgressGatewayBFDConfig `json:"bfd"`
	// egress policies
	// at least one policy must be specified
	Policies []VpcEgressGatewayPolicy `json:"policies,omitempty"`
	// optional node selector used to select the nodes where the workload will be running
	NodeSelector []VpcEgressGatewayNodeSelector `json:"nodeSelector,omitempty"`
}

type VpcEgressGatewaySelector struct {
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	PodSelector       *metav1.LabelSelector `json:"podSelector,omitempty"`
}

type VpcEgressGatewayBFDConfig struct {
	// whether to enable BFD
	// if set to true, the egress gateway will establish BFD session(s) with the VPC BFD LRP
	// the VPC's .spec.bfd.enabled must be set to true to enable BFD
	Enabled bool `json:"enabled"`
	// optional BFD minRX/minTX/multiplier
	MinRX      int32 `json:"minRX,omitempty"`
	MinTX      int32 `json:"minTX,omitempty"`
	Multiplier int32 `json:"multiplier,omitempty"`
}

type VpcEgressGatewayPolicy struct {
	// whether to enable SNAT/MASQUERADE for the egress traffic
	SNAT bool `json:"snat"`
	// CIDRs/subnets targeted by the egress traffic policy
	IPBlocks []string `json:"ipBlocks,omitempty"`
	Subnets  []string `json:"subnets,omitempty"`
}

type VpcEgressGatewayNodeSelector struct {
	MatchLabels      map[string]string                `json:"matchLabels,omitempty"`
	MatchExpressions []corev1.NodeSelectorRequirement `json:"matchExpressions,omitempty"`
	MatchFields      []corev1.NodeSelectorRequirement `json:"matchFields,omitempty"`
}

type VpcEgressGatewayStatus struct {
	// used by the scale subresource
	Replicas      int32  `json:"replicas,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`

	// whether the egress gateway is ready
	Ready bool  `json:"ready"`
	Phase Phase `json:"phase"`
	// internal/external IPs used by the workload
	InternalIPs []string   `json:"internalIPs,omitempty"`
	ExternalIPs []string   `json:"externalIPs,omitempty"`
	Conditions  Conditions `json:"conditions,omitempty"`

	// workload information
	Workload VpcEgressWorkload `json:"workload"`
}

type VpcEgressWorkload struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	// nodes where the workload is running
	Nodes []string `json:"nodes,omitempty"`
}

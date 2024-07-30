/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="ClusterName",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="KubernetesVersion",type="string",JSONPath=".spec.kubernetesVersion"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="FailureMessage",type="string",JSONPath=".status.failureMessage"

type AKSClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AKSClusterConfigSpec   `json:"spec"`
	Status AKSClusterConfigStatus `json:"status"`
}

// AKSClusterConfigSpec is the spec for a AKSClusterConfig resource
type AKSClusterConfigSpec struct {
	// Importer indicates that the cluster was imported.
	// +optional
	// +kubebuilder:default=false
	Imported bool `json:"imported" norman:"noupdate"`
	// Location specifies the region to create the private endpoint.
	ResourceLocation string `json:"resourceLocation" norman:"noupdate"`
	// ResourceGroup is the name of the Azure resource group for this AKS Cluster.
	// Immutable.
	ResourceGroup string `json:"resourceGroup" norman:"noupdate"`
	// AKS ClusterName allows you to specify the name of the AKS cluster in Azure.
	ClusterName string `json:"clusterName" norman:"noupdate"`
	// AzureCredentialSecret is the name of the secret containing the Azure credentials.
	AzureCredentialSecret string `json:"azureCredentialSecret"`
	// BaseURL is the Azure Resource Manager endpoint.
	// +optional
	BaseURL *string `json:"baseUrl" norman:"pointer"`
	// AuthBaseURL is the Azure Active Directory endpoint.
	// +optional
	AuthBaseURL *string `json:"authBaseUrl" norman:"pointer"`
	// NetworkPlugin used for building Kubernetes network.
	// Allowed values are "azure", "kubenet".
	// Immutable.
	// +kubebuilder:validation:Enum=azure;kubenet
	// +optional
	NetworkPlugin *string `json:"networkPlugin" norman:"pointer"`
	// VirualNetworkResourceGroup is the name of the Azure resource group for the VNet and Subnet.
	// +optional
	VirtualNetworkResourceGroup *string `json:"virtualNetworkResourceGroup" norman:"pointer"`
	// VirtualNetwork describes the vnet for the AKS cluster. Will be created if it does not exist.
	// +optional
	VirtualNetwork *string `json:"virtualNetwork" norman:"pointer"`
	// Subnet describes a subnet for an AKS cluster.
	Subnet *string `json:"subnet" norman:"pointer"`
	// NeworkDNSServiceIP is an IP address assigned to the Kubernetes DNS service.
	// It must be within the Kubernetes service address range specified in serviceCidr.
	// Immutable.
	// +optional
	NetworkDNSServiceIP *string `json:"dnsServiceIp" norman:"pointer"`
	// NetworkService CIDR is the network service cidr.
	NetworkServiceCIDR *string `json:"serviceCidr" norman:"pointer"`
	// NetworkDockerBridgeCIDR is the network docker bridge cidr.
	// Setting the dockerBridgeCidr field is no longer supported,
	// see https://github.com/Azure/AKS/issues/3534
	NetworkDockerBridgeCIDR *string `json:"dockerBridgeCidr" norman:"pointer"`
	// NetworkPodCIDR is the network pod cidr.
	NetworkPodCIDR *string `json:"podCidr" norman:"pointer"`
	// NodeResourceGroupName is the name of the resource group
	// containing cluster IaaS resources.
	// +optional
	NodeResourceGroup *string `json:"nodeResourceGroup,omitempty" norman:"pointer"`
	// Outbound configuration used by Nodes.
	// Immutable.
	// +kubebuilder:validation:Enum=loadBalancer;managedNATGateway;userAssignedNATGateway;userDefinedRouting
	// +optional
	OutboundType *string `json:"outboundType" norman:"pointer"`
	// LoadBalancerSKU is the SKU of the loadBalancer to be provisioned.
	// Immutable.
	// +kubebuilder:validation:Enum=Basic;Standard
	// +optional
	LoadBalancerSKU *string `json:"loadBalancerSku" norman:"pointer"`
	// NetworkPolicy used for building Kubernetes network.
	// Allowed values are "azure", "calico".
	// Immutable.
	// +kubebuilder:validation:Enum=azure;calico
	// +optional
	NetworkPolicy *string `json:"networkPolicy" norman:"pointer"`
	// LinuxAdminUsername is a string literal containing a linux admin username.
	// +optional
	LinuxAdminUsername *string `json:"linuxAdminUsername,omitempty" norman:"pointer"`
	// LinuxSSHPublicKey is a string literal containing a ssh public key.
	// +optional
	LinuxSSHPublicKey *string `json:"sshPublicKey,omitempty" norman:"pointer"`
	// DNSPrefix is the DNS prefix to use with hosted Kubernetes API server FQDN.
	DNSPrefix *string `json:"dnsPrefix,omitempty" norman:"pointer"`
	// Version defines the desired Kubernetes version.
	// +kubebuilder:validation:MinLength:=2
	KubernetesVersion *string `json:"kubernetesVersion" norman:"pointer"`
	// Tags is an optional set of tags to add to Azure resources managed by the Azure provider, in addition to the
	// ones added by default.
	// +optional
	Tags map[string]string `json:"tags"`
	// NodePools is a list of node pools associated with the AKS cluster.
	NodePools []AKSNodePool `json:"nodePools"`
	// PrivateCluster - Whether to create the cluster as a private cluster or not.
	// +optional
	PrivateCluster *bool `json:"privateCluster"`
	// PrivateDNSZone - Private dns zone mode for private cluster.
	// +kubebuilder:validation:Enum=System;None
	// +optional
	PrivateDNSZone *string `json:"privateDnsZone" norman:"pointer"`
	// AuthorizedIPRanges - Authorized IP Ranges to kubernetes API server.
	// +optional
	AuthorizedIPRanges *[]string `json:"authorizedIpRanges" norman:"pointer"`
	// HTTPApplicationRouting is enabling add-on for the cluster.
	// Immutable.
	// +optional
	HTTPApplicationRouting *bool `json:"httpApplicationRouting"`
	// Monitoring is enabling add-on for the AKS cluster.
	Monitoring *bool `json:"monitoring"`
	// LogAnalyticsWorkspaceResourceGroup is the name of the resource group for the Log Analytics Workspace.
	// +optional
	LogAnalyticsWorkspaceGroup *string `json:"logAnalyticsWorkspaceGroup" norman:"pointer"`
	// LogAnalyticsWorkspaceName is the name of the Log Analytics Workspace.
	// +optional
	LogAnalyticsWorkspaceName *string `json:"logAnalyticsWorkspaceName" norman:"pointer"`
	// ManagedIdentity - Should a managed identity be enabled or not?
	ManagedIdentity *bool `json:"managedIdentity" norman:"pointer"`
	// UserAssignedIdentity - User assigned identity to be used for the cluster.
	UserAssignedIdentity *string `json:"userAssignedIdentity" norman:"pointer"`
}

type AKSClusterConfigStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
	RBACEnabled    *bool  `json:"rbacEnabled"`
}

type AKSNodePool struct {
	// Name is the name of the node pool.
	Name *string `json:"name,omitempty" norman:"pointer"`
	// NodeCount is the number of nodes in the node pool.
	Count *int32 `json:"count,omitempty"`
	// MaxPods is the maximum number of pods that can run on each node.
	MaxPods *int32 `json:"maxPods,omitempty"`
	// VMSize is the size of the Virtual Machine.
	VMSize string `json:"vmSize,omitempty"`
	// OsDiskSizeGB is the disk size of the OS disk in GB.
	// +kubebuilder:validation:Minimum=0
	OsDiskSizeGB *int32 `json:"osDiskSizeGB,omitempty"`
	// OSDiskType is the type of the OS disk.
	// +kubebuilder:validation:Enum=Standard_LRS;Premium_LRS;StandardSSD_LRS;UltraSSD_LRS;Ephemeral;Managed
	OsDiskType string `json:"osDiskType,omitempty"`
	// Mode is the mode of the node pool.
	// +kubebuilder:validation:Enum=System;User
	Mode string `json:"mode,omitempty"`
	// OsType is the type of the OS.
	OsType string `json:"osType,omitempty"`
	// OrchestratorVersion is the version of the Kubernetes.
	// +kubebuilder:validation:MinLength:=2
	OrchestratorVersion *string `json:"orchestratorVersion,omitempty" norman:"pointer"`
	// AvailabilityZones is the list of availability zones.
	// +optional
	// +kubebuilder:validation:UniqueItems:=true
	AvailabilityZones *[]string `json:"availabilityZones,omitempty" norman:"pointer"`
	// MaxSurge is the maximum number of nodes that can be added to the node pool during an upgrade.
	// +optional
	MaxSurge *string `json:"maxSurge,omitempty"`
	// MaxCount is the maximum number of nodes in the node pool.
	// +kubebuilder:validation:Minimum=0
	MaxCount *int32 `json:"maxCount,omitempty"`
	// MinCount is the minimum number of nodes in the node pool.
	// +kubebuilder:validation:Minimum=0
	MinCount *int32 `json:"minCount,omitempty"`
	// EnableAutoScaling is whether to enable auto scaling or not.
	// +optional
	EnableAutoScaling *bool `json:"enableAutoScaling,omitempty"`
	// VnetSubnetID is the ID of the subnet.
	VnetSubnetID *string `json:"vnetSubnetID,omitempty" norman:"pointer"`
	// NodeLabels is the list of node labels.
	// +optional
	NodeLabels map[string]*string `json:"nodeLabels,omitempty"`
	// NodeTaints is the list of node taints.
	// +kubebuilder:validation:UniqueItems:=true
	// +optional
	NodeTaints *[]string `json:"nodeTaints,omitempty"`
}

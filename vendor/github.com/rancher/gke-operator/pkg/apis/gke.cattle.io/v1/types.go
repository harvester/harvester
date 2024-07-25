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
// +kubebuilder:printcolumn:name="ProjectID",type="string",JSONPath=".spec.projectID"
// +kubebuilder:printcolumn:name="ClusterName",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="KubernetesVersion",type="string",JSONPath=".spec.kubernetesVersion"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="FailureMessage",type="string",JSONPath=".status.failureMessage"

type GKEClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GKEClusterConfigSpec   `json:"spec"`
	Status GKEClusterConfigStatus `json:"status"`
}

// GKEClusterConfigSpec is the spec for a GKEClusterConfig resource
type GKEClusterConfigSpec struct {
	// Region is the GCP region where the cluster is located.
	// Required if Zone is not set.
	// +optional
	Region string `json:"region" norman:"noupdate"`

	// Zone is the GCP zone where the cluster is located.
	// Required if Region is not set.
	// +optional
	Zone string `json:"zone" norman:"noupdate"`

	// Imported indicates whether the cluster is imported.
	// +optional
	// +kubebuilder:default=false
	Imported bool `json:"imported" norman:"noupdate"`

	// Description is a human-readable description of the cluster.
	// +optional
	Description string `json:"description"`

	// Labels are user-defined key-value pairs for the cluster.
	// +optional
	Labels map[string]string `json:"labels"`

	// EnableKubernetesAlpha enables alpha features in Kubernetes.
	// +optional
	// +kubebuilder:default=false
	EnableKubernetesAlpha *bool `json:"enableKubernetesAlpha"`

	// ClusterAddons contains configuration for cluster add-ons.
	// +optional
	ClusterAddons *GKEClusterAddons `json:"clusterAddons"`

	// ClusterIpv4CidrBlock is the IPv4 CIDR block for the cluster.
	// +optional
	ClusterIpv4CidrBlock *string `json:"clusterIpv4Cidr" norman:"pointer"`

	// ProjectID is the ID of the GCP project where the cluster is created.
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectID"`

	// GoogleCredentialSecret is the name of the secret containing Google credentials.
	// +kubebuilder:validation:Required
	GoogleCredentialSecret string `json:"googleCredentialSecret"`

	// ClusterName is the name of the GKE cluster.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// KubernetesVersion is the version of Kubernetes to use.
	// +optional
	KubernetesVersion *string `json:"kubernetesVersion" norman:"pointer"`

	// LoggingService is the logging service to use.
	// +optional
	LoggingService *string `json:"loggingService" norman:"pointer"`

	// MonitoringService is the monitoring service to use.
	// +optional
	MonitoringService *string `json:"monitoringService" norman:"pointer"`

	// NodePools is a list of node pool configurations.
	// +optional
	NodePools []GKENodePoolConfig `json:"nodePools"`

	// Network is the name of the network to use, if specified.
	// +optional
	Network *string `json:"network,omitempty" norman:"pointer"`

	// Subnetwork is the name of the subnetwork to use, if specified.
	// +optional
	Subnetwork *string `json:"subnetwork,omitempty" norman:"pointer"`

	// NetworkPolicyEnabled enables network policy enforcement, if true.
	// +optional
	// +kubebuilder:default=false
	NetworkPolicyEnabled *bool `json:"networkPolicyEnabled,omitempty"`

	// PrivateClusterConfig contains private cluster configuration.
	// +optional
	PrivateClusterConfig *GKEPrivateClusterConfig `json:"privateClusterConfig,omitempty"`

	// IPAllocationPolicy is the IP allocation policy for the cluster.
	// +optional
	IPAllocationPolicy *GKEIPAllocationPolicy `json:"ipAllocationPolicy,omitempty"`

	// MasterAuthorizedNetworksConfig is the configuration for master authorized networks.
	// +optional
	MasterAuthorizedNetworksConfig *GKEMasterAuthorizedNetworksConfig `json:"masterAuthorizedNetworks,omitempty"`

	// Locations are the regions/zone where the cluster is available.
	// +optional
	Locations []string `json:"locations"`

	// MaintenanceWindow is the maintenance window configuration.
	// +optional
	MaintenanceWindow *string `json:"maintenanceWindow,omitempty" norman:"pointer"`

	// GKE Autopilot is a mode of operation in GKE in which Google manages your cluster configuration,
	// including your nodes, scaling, security, and other preconfigured settings.
	// +optional
	AutopilotConfig *GKEAutopilotConfig `json:"autopilotConfig,omitempty"`

	// The Customer Managed Encryption Key Config used to encrypt the boot disk attached
	// to each node in the node pool.
	// +optional
	CustomerManagedEncryptionKey *CMEKConfig `json:"customerManagedEncryptionKey,omitempty"`
}

type GKEIPAllocationPolicy struct {
	// ClusterIpv4CidrBlock is the IPv4 CIDR block for the cluster.
	// +optional
	ClusterIpv4CidrBlock string `json:"clusterIpv4CidrBlock,omitempty"`

	// ClusterSecondaryRangeName is the name of the secondary range for cluster pods.
	// +optional
	ClusterSecondaryRangeName string `json:"clusterSecondaryRangeName,omitempty"`

	// CreateSubnetwork indicates whether to create a subnetwork for the cluster.
	// +optional
	// +kubebuilder:default=false
	CreateSubnetwork bool `json:"createSubnetwork,omitempty"`

	// NodeIpv4CidrBlock is the IPv4 CIDR block for nodes.
	// +optional
	NodeIpv4CidrBlock string `json:"nodeIpv4CidrBlock,omitempty"`

	// ServicesIpv4CidrBlock is the IPv4 CIDR block for services.
	// +optional
	ServicesIpv4CidrBlock string `json:"servicesIpv4CidrBlock,omitempty"`

	// ServicesSecondaryRangeName is the name of the secondary range for services.
	// +optional
	ServicesSecondaryRangeName string `json:"servicesSecondaryRangeName,omitempty"`

	// SubnetworkName is the name of the subnetwork to use, if specified.
	// +optional
	SubnetworkName string `json:"subnetworkName,omitempty"`

	// UseIPAliases indicates whether to use IP aliases.
	// +optional
	// +kubebuilder:default=true
	UseIPAliases bool `json:"useIpAliases,omitempty"`
}

type GKEPrivateClusterConfig struct {
	// EnablePrivateEndpoint enables the private endpoint for the cluster.
	// +optional
	// +kubebuilder:default=false
	EnablePrivateEndpoint bool `json:"enablePrivateEndpoint,omitempty"`

	// EnablePrivateNodes enables private nodes for the cluster.
	// +optional
	// +kubebuilder:default=false
	EnablePrivateNodes bool `json:"enablePrivateNodes,omitempty"`

	// MasterIpv4CidrBlock is the IPv4 CIDR block for the master.
	// +kubebuilder:validation:Required
	MasterIpv4CidrBlock string `json:"masterIpv4CidrBlock,omitempty"`
}

type GKEClusterConfigStatus struct {
	// Phase represents the current phase of the cluster.
	// +optional
	Phase string `json:"phase"`

	// FailureMessage contains an optional failure message for the cluster.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`
}

type GKEClusterAddons struct {
	// HTTPLoadBalancing indicates whether HTTP load balancing is enabled.
	// +optional
	// +kubebuilder:default=false
	HTTPLoadBalancing bool `json:"httpLoadBalancing,omitempty"`

	// HorizontalPodAutoscaling indicates whether horizontal pod autoscaling is enabled.
	// +optional
	// +kubebuilder:default=false
	HorizontalPodAutoscaling bool `json:"horizontalPodAutoscaling,omitempty"`

	// NetworkPolicyConfig indicates whether network policy configuration is enabled.
	// +optional
	// +kubebuilder:default=false
	NetworkPolicyConfig bool `json:"networkPolicyConfig,omitempty"`
}

type GKENodePoolConfig struct {
	// Autoscaling specifies the autoscaling configuration for the node pool.
	// +optional
	Autoscaling *GKENodePoolAutoscaling `json:"autoscaling,omitempty"`

	// Config specifies the configuration for nodes in the node pool.
	// +optional
	Config *GKENodeConfig `json:"config,omitempty"`

	// InitialNodeCount is the initial number of nodes in the node pool.
	// +kubebuilder:validation:Required
	InitialNodeCount *int64 `json:"initialNodeCount,omitempty"`

	// MaxPodsConstraint is the maximum number of pods that can run on a node in the node pool.
	// +optional
	MaxPodsConstraint *int64 `json:"maxPodsConstraint,omitempty"`

	// Name is the name of the node pool.
	// +kubebuilder:validation:Required
	Name *string `json:"name,omitempty" norman:"pointer"`

	// Version is the Kubernetes version for the node pool.
	// +kubebuilder:validation:Required
	Version *string `json:"version,omitempty" norman:"pointer"`

	// Management specifies the management configuration for the node pool.
	// +optional
	Management *GKENodePoolManagement `json:"management,omitempty"`
}

type GKENodePoolAutoscaling struct {
	// Enabled indicates whether autoscaling is enabled for the node pool.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// MaxNodeCount is the maximum number of nodes in the node pool when autoscaling is enabled.
	// +optional
	MaxNodeCount int64 `json:"maxNodeCount,omitempty"`

	// MinNodeCount is the minimum number of nodes in the node pool when autoscaling is enabled.
	// +optional
	MinNodeCount int64 `json:"minNodeCount,omitempty"`
}

type GKENodeConfig struct {
	// BootDiskKmsKey is the Customer Managed Encryption Key used to encrypt
	// the boot disk attached to each node in the node pool. This should be
	// of the form
	// projects/[PROJECT_ID]/locations/[LOCATION]/keyRings/[RING_NAME]/cryptoKeys/[KEY_NAME]
	// +optional
	BootDiskKmsKey string `json:"bootDiskKmsKey,omitempty"`

	// DiskSizeGb is the size of the node's disk in gigabytes.
	// +optional
	DiskSizeGb int64 `json:"diskSizeGb,omitempty"`

	// DiskType is the type of disk to use for the node.
	// +optional
	DiskType string `json:"diskType,omitempty"`

	// ImageType is the type of image to use for the node.
	// +optional
	ImageType string `json:"imageType,omitempty"`

	// Labels are user-defined key-value pairs for the node.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// LocalSsdCount is the number of local SSDs to attach to the node.
	// +optional
	LocalSsdCount int64 `json:"localSsdCount,omitempty"`

	// MachineType is the type of machine for the node.
	// +optional
	MachineType string `json:"machineType,omitempty"`

	// OauthScopes are the OAuth scopes for the node.
	// +optional
	OauthScopes []string `json:"oauthScopes,omitempty"`

	// Preemptible indicates whether the node is preemptible.
	// +optional
	// +kubebuilder:default=false
	Preemptible bool `json:"preemptible,omitempty"`

	// Tags are the tags associated with the node.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Taints are the taints applied to the node.
	// +optional
	Taints []GKENodeTaintConfig `json:"taints,omitempty"`

	// ServiceAccount is the email address of the service account that is assigned to each node in the node pool.
	// If not specified, the default service account is used.
	// +optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

type GKENodeTaintConfig struct {
	// Effect is the effect of the taint, which can be NoSchedule or PreferNoSchedule or NoExecute.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NoSchedule;PreferNoSchedule;NoExecute
	Effect string `json:"effect,omitempty"`

	// Key is the key of the taint.
	// +kubebuilder:validation:Required
	Key string `json:"key,omitempty"`

	// Value is the value of the taint.
	// +kubebuilder:validation:Required
	Value string `json:"value,omitempty"`
}

type GKEMasterAuthorizedNetworksConfig struct {
	// CidrBlocks is an array of CIDR blocks that are authorized for access to the master.
	// +kubebuilder:validation:Required
	CidrBlocks []*GKECidrBlock `json:"cidrBlocks,omitempty"`

	// Enabled indicates whether master authorized networks are enabled.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

type GKECidrBlock struct {
	// CidrBlock is the CIDR block for the network.
	// +kubebuilder:validation:Required
	CidrBlock string `json:"cidrBlock,omitempty"`

	// DisplayName is a display name for the CIDR block.
	// +optional
	DisplayName string `json:"displayName,omitempty"`
}

type GKENodePoolManagement struct {
	// AutoRepair indicates whether auto repair is enabled for the node pool.
	// +optional
	// +kubebuilder:default=false
	AutoRepair bool `json:"autoRepair,omitempty"`

	// AutoUpgrade indicates whether auto upgrade is enabled for the node pool.
	// +optional
	// +kubebuilder:default=false
	AutoUpgrade bool `json:"autoUpgrade,omitempty"`
}

type GKEAutopilotConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// The Customer Managed Encryption Key Config struct
// used to provide ring name and key name for encryption
// the boot disk attached to each node in the node pool.
// +optional
type CMEKConfig struct {
	// KeyName is the name of the key to use for encryption.
	// It has to be provided together with RingName.
	KeyName string `json:"keyName,omitempty"`
	// RingName is the name of the ring to use for encryption.
	// It has to be provided together with KeyName.
	RingName string `json:"ringName,omitempty"`
}

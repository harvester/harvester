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

type GKEClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GKEClusterConfigSpec   `json:"spec"`
	Status GKEClusterConfigStatus `json:"status"`
}

// GKEClusterConfigSpec is the spec for a GKEClusterConfig resource
type GKEClusterConfigSpec struct {
	Region                         string                             `json:"region" norman:"noupdate"`
	Zone                           string                             `json:"zone" norman:"noupdate"`
	Imported                       bool                               `json:"imported" norman:"noupdate"`
	Description                    string                             `json:"description"`
	Labels                         map[string]string                  `json:"labels"`
	EnableKubernetesAlpha          *bool                              `json:"enableKubernetesAlpha"`
	ClusterAddons                  *GKEClusterAddons                  `json:"clusterAddons"`
	ClusterIpv4CidrBlock           *string                            `json:"clusterIpv4Cidr" norman:"pointer"`
	ProjectID                      string                             `json:"projectID"`
	GoogleCredentialSecret         string                             `json:"googleCredentialSecret"`
	ClusterName                    string                             `json:"clusterName"`
	KubernetesVersion              *string                            `json:"kubernetesVersion" norman:"pointer"`
	LoggingService                 *string                            `json:"loggingService" norman:"pointer"`
	MonitoringService              *string                            `json:"monitoringService" norman:"pointer"`
	NodePools                      []GKENodePoolConfig                `json:"nodePools"`
	Network                        *string                            `json:"network,omitempty" norman:"pointer"`
	Subnetwork                     *string                            `json:"subnetwork,omitempty" norman:"pointer"`
	NetworkPolicyEnabled           *bool                              `json:"networkPolicyEnabled,omitempty"`
	PrivateClusterConfig           *GKEPrivateClusterConfig           `json:"privateClusterConfig,omitempty"`
	IPAllocationPolicy             *GKEIPAllocationPolicy             `json:"ipAllocationPolicy,omitempty"`
	MasterAuthorizedNetworksConfig *GKEMasterAuthorizedNetworksConfig `json:"masterAuthorizedNetworks,omitempty"`
	Locations                      []string                           `json:"locations"`
	MaintenanceWindow              *string                            `json:"maintenanceWindow,omitempty" norman:"pointer"`
}

type GKEIPAllocationPolicy struct {
	ClusterIpv4CidrBlock       string `json:"clusterIpv4CidrBlock,omitempty"`
	ClusterSecondaryRangeName  string `json:"clusterSecondaryRangeName,omitempty"`
	CreateSubnetwork           bool   `json:"createSubnetwork,omitempty"`
	NodeIpv4CidrBlock          string `json:"nodeIpv4CidrBlock,omitempty"`
	ServicesIpv4CidrBlock      string `json:"servicesIpv4CidrBlock,omitempty"`
	ServicesSecondaryRangeName string `json:"servicesSecondaryRangeName,omitempty"`
	SubnetworkName             string `json:"subnetworkName,omitempty"`
	UseIPAliases               bool   `json:"useIpAliases,omitempty"`
}

type GKEPrivateClusterConfig struct {
	EnablePrivateEndpoint bool   `json:"enablePrivateEndpoint,omitempty"`
	EnablePrivateNodes    bool   `json:"enablePrivateNodes,omitempty"`
	MasterIpv4CidrBlock   string `json:"masterIpv4CidrBlock,omitempty"`
}

type GKEClusterConfigStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
}

type GKEClusterAddons struct {
	HTTPLoadBalancing        bool `json:"httpLoadBalancing,omitempty"`
	HorizontalPodAutoscaling bool `json:"horizontalPodAutoscaling,omitempty"`
	NetworkPolicyConfig      bool `json:"networkPolicyConfig,omitempty"`
}

type GKENodePoolConfig struct {
	Autoscaling       *GKENodePoolAutoscaling `json:"autoscaling,omitempty"`
	Config            *GKENodeConfig          `json:"config,omitempty"`
	InitialNodeCount  *int64                  `json:"initialNodeCount,omitempty"`
	MaxPodsConstraint *int64                  `json:"maxPodsConstraint,omitempty"`
	Name              *string                 `json:"name,omitempty" norman:"pointer"`
	Version           *string                 `json:"version,omitempty" norman:"pointer"`
	Management        *GKENodePoolManagement  `json:"management,omitempty"`
}

type GKENodePoolAutoscaling struct {
	Enabled      bool  `json:"enabled,omitempty"`
	MaxNodeCount int64 `json:"maxNodeCount,omitempty"`
	MinNodeCount int64 `json:"minNodeCount,omitempty"`
}

type GKENodeConfig struct {
	DiskSizeGb    int64                `json:"diskSizeGb,omitempty"`
	DiskType      string               `json:"diskType,omitempty"`
	ImageType     string               `json:"imageType,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty"`
	LocalSsdCount int64                `json:"localSsdCount,omitempty"`
	MachineType   string               `json:"machineType,omitempty"`
	OauthScopes   []string             `json:"oauthScopes,omitempty"`
	Preemptible   bool                 `json:"preemptible,omitempty"`
	Tags          []string             `json:"tags,omitempty"`
	Taints        []GKENodeTaintConfig `json:"taints,omitempty"`
}

type GKENodeTaintConfig struct {
	Effect string `json:"effect,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

type GKEMasterAuthorizedNetworksConfig struct {
	CidrBlocks []*GKECidrBlock `json:"cidrBlocks,omitempty"`
	Enabled    bool            `json:"enabled,omitempty"`
}

type GKECidrBlock struct {
	CidrBlock   string `json:"cidrBlock,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type GKENodePoolManagement struct {
	AutoRepair  bool `json:"autoRepair,omitempty"`
	AutoUpgrade bool `json:"autoUpgrade,omitempty"`
}

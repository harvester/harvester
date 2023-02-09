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

type EKSClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSClusterConfigSpec   `json:"spec"`
	Status EKSClusterConfigStatus `json:"status"`
}

// EKSClusterConfigSpec is the spec for a EKSClusterConfig resource
type EKSClusterConfigSpec struct {
	AmazonCredentialSecret string            `json:"amazonCredentialSecret"`
	DisplayName            string            `json:"displayName" norman:"noupdate"`
	Region                 string            `json:"region" norman:"noupdate"`
	Imported               bool              `json:"imported" norman:"noupdate"`
	KubernetesVersion      *string           `json:"kubernetesVersion" norman:"pointer"`
	Tags                   map[string]string `json:"tags"`
	SecretsEncryption      *bool             `json:"secretsEncryption" norman:"noupdate"`
	KmsKey                 *string           `json:"kmsKey" norman:"noupdate,pointer"`
	PublicAccess           *bool             `json:"publicAccess"`
	PrivateAccess          *bool             `json:"privateAccess"`
	PublicAccessSources    []string          `json:"publicAccessSources"`
	LoggingTypes           []string          `json:"loggingTypes"`
	Subnets                []string          `json:"subnets" norman:"noupdate"`
	SecurityGroups         []string          `json:"securityGroups" norman:"noupdate"`
	ServiceRole            *string           `json:"serviceRole" norman:"noupdate,pointer"`
	NodeGroups             []NodeGroup       `json:"nodeGroups"`
}

type EKSClusterConfigStatus struct {
	Phase                         string            `json:"phase"`
	VirtualNetwork                string            `json:"virtualNetwork"`
	Subnets                       []string          `json:"subnets"`
	SecurityGroups                []string          `json:"securityGroups"`
	ManagedLaunchTemplateID       string            `json:"managedLaunchTemplateID"`
	ManagedLaunchTemplateVersions map[string]string `json:"managedLaunchTemplateVersions"`
	TemplateVersionsToDelete      []string          `json:"templateVersionsToDelete"`
	// describes how the above network fields were provided. Valid values are provided and generated
	NetworkFieldsSource string `json:"networkFieldsSource"`
	FailureMessage      string `json:"failureMessage"`
}

type NodeGroup struct {
	Gpu                  *bool              `json:"gpu"`
	ImageID              *string            `json:"imageId" norman:"pointer"`
	NodegroupName        *string            `json:"nodegroupName" norman:"required,pointer" wrangler:"required"`
	DiskSize             *int64             `json:"diskSize"`
	InstanceType         *string            `json:"instanceType" norman:"pointer"`
	Labels               map[string]*string `json:"labels"`
	Ec2SshKey            *string            `json:"ec2SshKey" norman:"pointer"`
	DesiredSize          *int64             `json:"desiredSize"`
	MaxSize              *int64             `json:"maxSize"`
	MinSize              *int64             `json:"minSize"`
	Subnets              []string           `json:"subnets"`
	Tags                 map[string]*string `json:"tags"`
	ResourceTags         map[string]*string `json:"resourceTags"`
	UserData             *string            `json:"userData" norman:"pointer"`
	Version              *string            `json:"version" norman:"pointer"`
	LaunchTemplate       *LaunchTemplate    `json:"launchTemplate"`
	RequestSpotInstances *bool              `json:"requestSpotInstances"`
	SpotInstanceTypes    []*string          `json:"spotInstanceTypes"`
}

type LaunchTemplate struct {
	ID      *string `json:"id" norman:"pointer"`
	Name    *string `json:"name" norman:"pointer"`
	Version *int64  `json:"version"`
}

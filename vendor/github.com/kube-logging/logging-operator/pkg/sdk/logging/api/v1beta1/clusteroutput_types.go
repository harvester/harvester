// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +name:"ClusterOutput"
// +weight:"200"
type _hugoClusterOutput interface{} //nolint:deadcode,unused

// +name:"ClusterOutput"
// +version:"v1beta1"
// +description:"ClusterOutput is the Schema for the clusteroutputs API"
type _metaClusterOutput interface{} //nolint:deadcode,unused

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the output active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// ClusterOutput is the Schema for the clusteroutputs API
type ClusterOutput struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterOutputSpec `json:"spec"`
	Status OutputStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:generate=true

// ClusterOutputSpec contains Kubernetes spec for ClusterOutput
type ClusterOutputSpec struct {
	OutputSpec        `json:",inline"`
	EnabledNamespaces []string `json:"enabledNamespaces,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterOutputList contains a list of ClusterOutput
type ClusterOutputList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterOutput `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterOutput{}, &ClusterOutputList{})
}

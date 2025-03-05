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

// +name:"FluentdConfig"
// +weight:"200"
type _hugoFluent interface{} //nolint:deadcode,unused

// +name:"Fluent"
// +version:"v1beta1"
// +description:"FluentdConfig is a reference to the desired Fluentd state"
type _metaFluentdConfig interface{} //nolint:deadcode,unused

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the fluentd configuration active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

type FluentdConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FluentdSpec         `json:"spec,omitempty"`
	Status FluentdConfigStatus `json:"status,omitempty"`
}

type FluentdConfigStatus struct {
	Logging       string   `json:"logging,omitempty"`
	Active        *bool    `json:"active,omitempty"`
	Problems      []string `json:"problems,omitempty"`
	ProblemsCount int      `json:"problemsCount,omitempty"`
}

// +kubebuilder:object:root=true

type FluentdConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FluentdConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FluentdConfig{}, &FluentdConfigList{})
}

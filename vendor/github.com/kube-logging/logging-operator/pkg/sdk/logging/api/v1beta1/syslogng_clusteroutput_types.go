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

// +name:"SyslogNGClusterOutput"
// +weight:"200"
type _hugoSyslogNGClusterOutput interface{} //nolint:deadcode,unused

// +name:"SyslogNGClusterOutput"
// +version:"v1beta1"
// +description:"SyslogNGClusterOutput is the Schema for the syslog-ng clusteroutputs API"
type _metaSyslogNGClusterOutput interface{} //nolint:deadcode,unused

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the output active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// SyslogNGClusterOutput is the Schema for the syslog-ng clusteroutputs API
type SyslogNGClusterOutput struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyslogNGClusterOutputSpec `json:"spec"`
	Status SyslogNGOutputStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:generate=true

// SyslogNGClusterOutputSpec contains Kubernetes spec for SyslogNGClusterOutput
type SyslogNGClusterOutputSpec struct {
	SyslogNGOutputSpec `json:",inline"`
	EnabledNamespaces  []string `json:"enabledNamespaces,omitempty"`
}

// +kubebuilder:object:root=true

// SyslogNGClusterOutputList contains a list of SyslogNGClusterOutput
type SyslogNGClusterOutputList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyslogNGClusterOutput `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyslogNGClusterOutput{}, &SyslogNGClusterOutputList{})
}

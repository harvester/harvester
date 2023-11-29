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

// +name:"SyslogNGClusterFlow"
// +weight:"200"
type _hugoSyslogNGClusterFlow interface{} //nolint:deadcode,unused

// +name:"SyslogNGClusterFlow"
// +version:"v1beta1"
// +description:"SyslogNGClusterFlow is the Schema for the syslog-ng clusterflows API"
type _metaSyslogNGClusterFlow interface{} //nolint:deadcode,unused

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the flow active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// SyslogNGClusterFlow is the Schema for the syslog-ng clusterflows API
type SyslogNGClusterFlow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyslogNGClusterFlowSpec `json:"spec,omitempty"`
	Status SyslogNGFlowStatus      `json:"status,omitempty"`
}

// SyslogNGClusterFlowSpec is the Kubernetes spec for Flows
type SyslogNGClusterFlowSpec struct {
	Match            *SyslogNGMatch   `json:"match,omitempty"`
	Filters          []SyslogNGFilter `json:"filters,omitempty"`
	LoggingRef       string           `json:"loggingRef,omitempty"`
	GlobalOutputRefs []string         `json:"globalOutputRefs,omitempty"`
}

type SyslogNGClusterMatch SyslogNGMatch

// +kubebuilder:object:root=true

// SyslogNGClusterFlowList contains a list of SyslogNGClusterFlow
type SyslogNGClusterFlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyslogNGClusterFlow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyslogNGClusterFlow{}, &SyslogNGClusterFlowList{})
}

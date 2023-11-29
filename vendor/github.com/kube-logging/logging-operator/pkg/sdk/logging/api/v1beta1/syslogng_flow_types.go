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
	filter "github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +name:"SyslogNGFlowSpec"
// +weight:"200"
type _hugoSyslogNGFlowSpec interface{} //nolint:deadcode,unused

// +name:"SyslogNGFlowSpec"
// +version:"v1beta1"
// +description:"SyslogNGFlowSpec is the Kubernetes spec for SyslogNGFlows"
type _metaSyslogNGFlowSpec interface{} //nolint:deadcode,unused

// SyslogNGFlowSpec is the Kubernetes spec for SyslogNGFlows
type SyslogNGFlowSpec struct {
	Match            *SyslogNGMatch   `json:"match,omitempty"`
	Filters          []SyslogNGFilter `json:"filters,omitempty"`
	LoggingRef       string           `json:"loggingRef,omitempty"`
	GlobalOutputRefs []string         `json:"globalOutputRefs,omitempty"`
	LocalOutputRefs  []string         `json:"localOutputRefs,omitempty"`
}

type SyslogNGMatch filter.MatchExpr

// IsEmpty returns true if the match is not specified, i.e. empty.
func (m *SyslogNGMatch) IsEmpty() bool {
	return (*filter.MatchExpr)(m).IsEmpty()
}

// Filter definition for SyslogNGFlowSpec
type SyslogNGFilter struct {
	ID      string                 `json:"id,omitempty" syslog-ng:"ignore"`
	Match   *filter.MatchConfig    `json:"match,omitempty" syslog-ng:"xform-kind=filter"`
	Rewrite []filter.RewriteConfig `json:"rewrite,omitempty" syslog-ng:"xform-kind=rewrite"`
	Parser  *filter.ParserConfig   `json:"parser,omitempty" syslog-ng:"xform-kind=parser"`
}

type SyslogNGFlowStatus FlowStatus

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the flow active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// Flow Kubernetes object
type SyslogNGFlow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyslogNGFlowSpec   `json:"spec,omitempty"`
	Status SyslogNGFlowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FlowList contains a list of Flow
type SyslogNGFlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyslogNGFlow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyslogNGFlow{}, &SyslogNGFlowList{})
}

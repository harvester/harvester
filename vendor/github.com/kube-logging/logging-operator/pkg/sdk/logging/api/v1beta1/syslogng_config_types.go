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

// +name:"SyslogNGConfig"
// +weight:"200"
type _hugoSyslogNGConfig interface{} //nolint:deadcode,unused

// +name:"SyslogNG"
// +version:"v1beta1"
// +description:"SyslogNGConfig is a standalone reference to the desired SyslogNG configuration"
type _metaSyslogNGConfig interface{} //nolint:deadcode,unused

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

type SyslogNGConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyslogNGSpec         `json:"spec,omitempty"`
	Status SyslogNGConfigStatus `json:"status,omitempty"`
}

type SyslogNGConfigStatus struct {
	Logging       string   `json:"logging,omitempty"`
	Active        *bool    `json:"active,omitempty"`
	Problems      []string `json:"problems,omitempty"`
	ProblemsCount int      `json:"problemsCount,omitempty"`
}

// +kubebuilder:object:root=true

type SyslogNGConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyslogNGConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyslogNGConfig{}, &SyslogNGConfigList{})
}

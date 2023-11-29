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
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/filter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +name:"FlowSpec"
// +weight:"200"
type _hugoFlowSpec interface{} //nolint:deadcode,unused

// +name:"FlowSpec"
// +version:"v1beta1"
// +description:"FlowSpec is the Kubernetes spec for Flows"
type _metaFlowSpec interface{} //nolint:deadcode,unused

// FlowSpec is the Kubernetes spec for Flows
type FlowSpec struct {
	// Deprecated
	Selectors  map[string]string `json:"selectors,omitempty"`
	Match      []Match           `json:"match,omitempty"`
	Filters    []Filter          `json:"filters,omitempty"`
	LoggingRef string            `json:"loggingRef,omitempty"`
	// Deprecated
	OutputRefs       []string `json:"outputRefs,omitempty"`
	GlobalOutputRefs []string `json:"globalOutputRefs,omitempty"`
	LocalOutputRefs  []string `json:"localOutputRefs,omitempty"`
}

type Match struct {
	*Select  `json:"select,omitempty"`
	*Exclude `json:"exclude,omitempty"`
}

type Select struct {
	Labels         map[string]string `json:"labels,omitempty"`
	Hosts          []string          `json:"hosts,omitempty"`
	ContainerNames []string          `json:"container_names,omitempty"`
}

type Exclude struct {
	Labels         map[string]string `json:"labels,omitempty"`
	Hosts          []string          `json:"hosts,omitempty"`
	ContainerNames []string          `json:"container_names,omitempty"`
}

// Filter definition for FlowSpec
type Filter struct {
	StdOut              *filter.StdOutFilterConfig        `json:"stdout,omitempty"`
	Parser              *filter.ParserConfig              `json:"parser,omitempty"`
	TagNormaliser       *filter.TagNormaliser             `json:"tag_normaliser,omitempty"`
	Dedot               *filter.DedotFilterConfig         `json:"dedot,omitempty"`
	ElasticGenId        *filter.ElasticsearchGenId        `json:"elasticsearch_genid,omitempty"`
	RecordTransformer   *filter.RecordTransformer         `json:"record_transformer,omitempty"`
	RecordModifier      *filter.RecordModifier            `json:"record_modifier,omitempty"`
	GeoIP               *filter.GeoIP                     `json:"geoip,omitempty"`
	Concat              *filter.Concat                    `json:"concat,omitempty"`
	DetectExceptions    *filter.DetectExceptions          `json:"detectExceptions,omitempty"`
	Grep                *filter.GrepConfig                `json:"grep,omitempty"`
	Prometheus          *filter.PrometheusConfig          `json:"prometheus,omitempty"`
	Throttle            *filter.Throttle                  `json:"throttle,omitempty"`
	SumoLogic           *filter.SumoLogic                 `json:"sumologic,omitempty"`
	EnhanceK8s          *filter.EnhanceK8s                `json:"enhanceK8s,omitempty"`
	KubeEventsTimestamp *filter.KubeEventsTimestampConfig `json:"kube_events_timestamp,omitempty"`
}

// FlowStatus defines the observed state of Flow
type FlowStatus struct {
	Active        *bool    `json:"active,omitempty"`
	Problems      []string `json:"problems,omitempty"`
	ProblemsCount int      `json:"problemsCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=logging-all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Is the flow active?"
// +kubebuilder:printcolumn:name="Problems",type="integer",JSONPath=".status.problemsCount",description="Number of problems"
// +kubebuilder:storageversion

// Flow Kubernetes object
type Flow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlowSpec   `json:"spec,omitempty"`
	Status FlowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FlowList contains a list of Flow
type FlowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Flow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Flow{}, &FlowList{})
}

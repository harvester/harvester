// Copyright © 2020 Banzai Cloud
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

package filter

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"SumoLogic"
// +weight:"200"
type _hugoSumoLogic interface{} //nolint:deadcode,unused

// +docName:"Sumo Logic collection solution for Kubernetes"
// More info at https://github.com/SumoLogic/sumologic-kubernetes-collection
type _docSumologic interface{} //nolint:deadcode,unused

// +name:"SumoLogic"
// +url:"https://github.com/SumoLogic/sumologic-kubernetes-collection"
// +version:"2.3.1"
// +description:"Sumo Logic collection solution for Kubernetes"
// +status:"GA"
type _metaSumologic interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type SumoLogic struct {
	// Source Category (default: "%{namespace}/%{pod_name}")
	SourceCategory string `json:"source_category,omitempty"`
	// Source Category Replace Dash (default: "/")
	SourceCategoryReplaceDash string `json:"source_category_replace_dash,omitempty"`
	// Source Category Prefix (default: kubernetes/)
	SourceCategoryPrefix string `json:"source_category_prefix,omitempty"`
	// Source Name (default: "%{namespace}.%{pod}.%{container}")
	SourceName string `json:"source_name,omitempty"`
	// Log Format (default: json)
	LogFormat string `json:"log_format,omitempty"`
	// Source Host (default: "")
	SourceHost string `json:"source_host,omitempty"`
	// Exclude Container Regex (default: "")
	ExcludeContainerRegex string `json:"exclude_container_regex,omitempty"`
	// Exclude Facility Regex (default: "")
	ExcludeFacilityRegex string `json:"exclude_facility_regex,omitempty"`
	// Exclude Host Regex (default: "")
	ExcludeHostRegex string `json:"exclude_host_regex,omitempty"`
	// Exclude Namespace Regex (default: "")
	ExcludeNamespaceRegex string `json:"exclude_namespace_regex,omitempty"`
	// Exclude Pod Regex (default: "")
	ExcludePodRegex string `json:"exclude_pod_regex,omitempty"`
	// Exclude Priority Regex (default: "")
	ExcludePriorityRegex string `json:"exclude_priority_regex,omitempty"`
	// Exclude Unit Regex (default: "")
	ExcludeUnitRegex string `json:"exclude_unit_regex,omitempty"`
	// Tracing Format (default: false)
	TracingFormat *bool `json:"tracing_format,omitempty"`
	// Tracing Namespace (default: "namespace")
	TracingNamespace string `json:"tracing_namespace,omitempty"`
	// Tracing Pod (default: "pod")
	TracingPod string `json:"tracing_pod,omitempty"`
	// Tracing Pod ID (default: "pod_id")
	TracingPodId string `json:"tracing_pod_id,omitempty"`
	// Tracing Container Name (default: "container_name")
	TracingContainerName string `json:"tracing_container_name,omitempty"`
	// Tracing Host (default: "hostname")
	TracingHost string `json:"tracing_host,omitempty"`
	// Tracing Label Prefix (default: "pod_label_")
	TracingLabelPrefix string `json:"tracing_label_prefix,omitempty"`
	// Tracing Annotation Prefix (default: "pod_annotation_")
	TracingAnnotationPrefix string `json:"tracing_annotation_prefix,omitempty"`
	// Source HostKey Name (default: "_sourceHost")
	SourceHostKeyName string `json:"source_host_key_name,omitempty"`
	// Source CategoryKey Name (default: "_sourceCategory")
	SourceCategoryKeyName string `json:"source_category_key_name,omitempty"`
	// Source NameKey Name (default: "_sourceName")
	SourceNameKeyName string `json:"source_name_key_name,omitempty"`
	// CollectorKey Name (default: "_collector")
	CollectorKeyName string `json:"collector_key_name,omitempty"`
	// Collector Value (default: "undefined")
	CollectorValue string `json:"collector_value,omitempty"`
}

// ## Example `Parser` filter configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Flow
// metadata:
//
//	name: demo-flow
//
// spec:
//
//	filters:
//	  - sumologic:
//	      source_name: "elso"
//	selectors: {}
//	localOutputRefs:
//	  - demo-output
//
// ```
//
// #### Fluentd Config Result
// ```yaml
// <filter **>
//
//	@type kubernetes_sumologic
//	@id test_sumologic
//	source_name elso
//
// </filter>
// ```
type _expSumologic interface{} //nolint:deadcode,unused

func (s *SumoLogic) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "kubernetes_sumologic"
	sumologic := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	sumoLogicConfig := s.DeepCopy()
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(sumoLogicConfig); err != nil {
		return nil, err
	} else {
		sumologic.Params = params
	}
	return sumologic, nil
}

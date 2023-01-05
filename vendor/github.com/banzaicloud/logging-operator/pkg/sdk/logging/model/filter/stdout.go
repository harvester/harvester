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

package filter

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"StdOut"
// +weight:"200"
type _hugoStdOut interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Stdout Filter](https://docs.fluentd.org/filter/stdout)"
// Fluentd Filter plugin to print events to stdout
type _docStdOut interface{} //nolint:deadcode,unused

// +name:"Stdout"
// +url:"https://docs.fluentd.org/filter/stdout"
// +version:"more info"
// +description:"Prints events to stdout"
// +status:"GA"
type _metaStdOut interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type StdOutFilterConfig struct {
	// This is the option of stdout format.
	OutputType string `json:"output_type,omitempty"`
}

// ## Example `StdOut` filter configurations
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
//	  - stdout:
//	      output_type: json
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
//	@type stdout
//	@id test_stdout
//	output_type json
//
// </filter>
// ```
type _expStdOut interface{} //nolint:deadcode,unused

func NewStdOutFilterConfig() *StdOutFilterConfig {
	return &StdOutFilterConfig{}
}

func (c *StdOutFilterConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "stdout"
	return types.NewFlatDirective(types.PluginMeta{
		Type:      pluginType,
		Directive: "filter",
		Tag:       "**",
		Id:        id,
	}, c, secretLoader)
}

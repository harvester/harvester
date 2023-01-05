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

// +name:"Dedot"
// +weight:"200"
type _hugoDedot interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Dedot Filter](https://github.com/lunardial/fluent-plugin-dedot_filter)"
// Fluentd Filter plugin to de-dot field name for elasticsearch.
type _docDedot interface{} //nolint:deadcode,unused

// +name:"Dedot"
// +url:"https://github.com/lunardial/fluent-plugin-dedot_filter"
// +version:"1.0.0"
// +description:"Concatenate multiline log separated in multiple events"
// +status:"GA"
type _metaDedot interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type DedotFilterConfig struct {
	// Will cause the plugin to recurse through nested structures (hashes and arrays), and remove dots in those key-names too.(default: false)
	Nested bool `json:"de_dot_nested,omitempty"`
	// Separator (default:_)
	Separator string `json:"de_dot_separator,omitempty"`
}

// ## Example `Dedot` filter configurations
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
//	  - dedot:
//	      de_dot_separator: "-"
//	      de_dot_nested: true
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
//	@type dedot
//	@id test_dedot
//	de_dot_nested true
//	de_dot_separator -
//
// </filter>
// ```
type _expDedot interface{} //nolint:deadcode,unused

func NewDedotFilterConfig() *DedotFilterConfig {
	return &DedotFilterConfig{}
}

func (c *DedotFilterConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "dedot"
	return types.NewFlatDirective(types.PluginMeta{
		Type:      pluginType,
		Directive: "filter",
		Tag:       "**",
		Id:        id,
	}, c, secretLoader)
}

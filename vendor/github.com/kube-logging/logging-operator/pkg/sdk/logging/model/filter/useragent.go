// Copyright © 2023 Kube logging authors
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
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"User Agent"
// +weight:"200"
type _hugoUserAgent interface{} //nolint:deadcode,unused

// +docName:"Fluentd UserAgent filter"
// Fluentd Filter plugin to parse user-agent
// More information at https://github.com/bungoume/fluent-plugin-ua-parser
type _docUserAgent interface{} //nolint:deadcode,unused

// +name:"UserAgent"
// +url:"https://github.com/bungoume/fluent-plugin-ua-parser"
// +version:"1.2.0"
// +description:"Fluentd UserAgent filter"
// +status:"GA"
type _metaUserAgent interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type UserAgent struct {
	// Target key name (default: user_agent)
	KeyName string `json:"key_name,omitempty"`
	// Delete input key (default: false)
	DeleteKey bool `json:"delete_key,omitempty"`
	// Output prefix key name (default: ua)
	OutKey string `json:"out_key,omitempty"`
	// Join hashed data by '_' (default: false)
	Flatten bool `json:"flatten,omitempty"`
}

//
/*
## Example `UserAgent` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - useragent:
        key_name: my_agent
        delete_key: true
        out_key: ua_fields
        flatten: true
  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<filter **>
  @type ua_parser
  @id test_useragent
  key_name my_agent
  delete_key true
  out_key ua_fields
  flatten true
</filter>
{{</ highlight >}}
*/
type _expUserAgent interface{} //nolint:deadcode,unused

func (g *UserAgent) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "ua_parser"
	userAgent := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(g); err != nil {
		return nil, err
	} else {
		userAgent.Params = params
	}
	return userAgent, nil
}

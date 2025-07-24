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

package output

import (
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Null"
// +weight:"200"
type _hugoNull interface{} //nolint:deadcode,unused

// +docName:"Null output plugin for Fluentd"
//
/*
For details, see [https://docs.fluentd.org/output/null](https://docs.fluentd.org/output/null).

## Example output configurations

```yaml
spec:
  nullout:
    never_flush: false
```
*/
type _docNull interface{} //nolint:deadcode,unused

// +name:"Null"
// +url:"https://docs.fluentd.org/output/null"
// +version:"more info"
// +description:"Null output plugin just throws away events."
// +status:"GA"
type _metaNull interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

type NullOutputConfig struct {
	// The parameter for testing to simulate the output plugin that never succeeds to flush.
	NeverFlush *bool `json:"never_flush,omitempty"`
}

func NewNullOutputConfig() *NullOutputConfig {
	return &NullOutputConfig{}
}

func (c *NullOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "null"
	null := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(c); err != nil {
		return nil, err
	} else {
		null.Params = params
	}

	return null, nil
}

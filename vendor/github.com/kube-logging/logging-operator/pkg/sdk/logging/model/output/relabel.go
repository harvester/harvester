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

package output

import (
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Relabel"
// +weight:"200"
type _hugoRelabel interface{} //nolint:deadcode,unused

// +docName:"Relabel"
/*
Available in Logging Operator version 4.2 and later.

The relabel output uses the [relabel output plugin of Fluentd](https://docs.fluentd.org/output/relabel) to route events back to a specific Flow, where they can be processed again.

This is useful, for example, if you need to preprocess a subset of logs differently, but then do the same processing on all messages at the end. In this case, you can create multiple flows for preprocessing based on specific log matchers and then aggregate everything into a single final flow for postprocessing.

The value of the `label` parameter of the relabel output must be the same as the value of the `flowLabel` parameter of the Flow (or ClusterFlow) where you want to send the messages.

For example:

```yaml
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterOutput
metadata:
  name: final-relabel
spec:
  relabel:
    label: '@final-flow'
---
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: serviceFlow1
  namespace: namespace1
spec:
  filters: []
  globalOutputRefs:
  - final-relabel
  match:
  - select:
      labels:
        app: service1
---
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: serviceFlow2
  namespace: namespace2
spec:
  filters: []
  globalOutputRefs:
  - final-relabel
  match:
  - select:
      labels:
        app: service2
---
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterFlow
metadata:
  name: final-flow
spec:
  flowLabel: '@final-flow'
  includeLabelInRouter: false
  filters: []
```

Using the relabel output also makes it possible to pass the messages emitted by the {{% xref "/docs/configuration/plugins/filters/concat.md" %}} plugin in case of a timeout. Set the `timeout_label` of the concat plugin to the flowLabel of the flow where you want to send the timeout messages.
*/
type _docRelabel interface{} //nolint:deadcode,unused

// +name:"Relabel"
// +url:"https://docs.fluentd.org/output/relabel"
// +version:"more info"
// +description:"Relabel output plugin re-labels events."
// +status:"GA"
type _metaRelabel interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type RelabelOutputConfig struct {
	// Specifies new label for events
	Label string `json:"label"`
}

func (c *RelabelOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "relabel"
	relabel := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
			Label:     c.Label,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(c); err != nil {
		return nil, err
	} else {
		relabel.Params = params
		delete(relabel.Params, "label")
	}
	return relabel, nil
}

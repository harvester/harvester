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
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Kubernetes Events Timestamp"
// +weight:"200"
type _hugoKubeEventsTimestamp interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Kubernetes Events Timestamp Filter](https://github.com/kube-logging/fluentd-filter-kube-events-timestamp)"
// Fluentd Filter plugin to select particular timestamp into an additional field
type _docKubeEventsTimestamp interface{} //nolint:deadcode,unused

// +name:"Kubernetes Events Timestamp"
// +url:"https://github.com/kube-logging/fluentd-filter-kube-events-timestamp"
// +version:"0.1.4"
// +description:"Fluentd Filter plugin to select particular timestamp into an additional field"
// +status:"GA"
type _metaKubeEventsTimestamp interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type KubeEventsTimestampConfig struct {
	// Time field names in order of relevance (default: event.eventTime, event.lastTimestamp, event.firstTimestamp)
	TimestampFields []string `json:"timestamp_fields,omitempty"`
	// Added time field name (default: triggerts)
	MappedTimeKey string `json:"mapped_time_key,omitempty"`
}

//
/*
## Example `Kubernetes Events Timestamp` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: es-flow
spec:
  filters:
    - kube_events_timestamp:
        timestamp_fields:
          - "event.eventTime"
          - "event.lastTimestamp"
          - "event.firstTimestamp"
        mapped_time_key: mytimefield
  selectors: {}
  localOutputRefs:
    - es-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
 <filter **>
 @type kube_events_timestamp
 @id test-kube-events-timestamp
 timestamp_fields ["event.eventTime","event.lastTimestamp","event.firstTimestamp"]
 mapped_time_key mytimefield
 </filter>
{{</ highlight >}}
*/
type _expKubeEventsTimestamp interface{} //nolint:deadcode,unused

func NewKubeEventsTimestampConfig() *KubeEventsTimestampConfig {
	return &KubeEventsTimestampConfig{}
}

func (c *KubeEventsTimestampConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "kube_events_timestamp"

	return types.NewFlatDirective(types.PluginMeta{
		Type:      pluginType,
		Directive: "filter",
		Tag:       "**",
		Id:        id,
	}, c, secretLoader)
}

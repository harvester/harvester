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

// +name:"Throttle"
// +weight:"200"
type _hugoThrottle interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Throttle Filter](https://github.com/rubrikinc/fluent-plugin-throttle)"
// A sentry plugin to throttle logs. Logs are grouped by a configurable key. When a group exceeds a configuration rate, logs are dropped for this group.
type _docThrottle interface{} //nolint:deadcode,unused

// +name:"Throttle"
// +url:"https://github.com/rubrikinc/fluent-plugin-throttle"
// +version:"0.0.5"
// +description:"A sentry plugin to throttle logs. Logs are grouped by a configurable key. When a group exceeds a configuration rate, logs are dropped for this group."
// +status:"GA"
type _metaThrottle interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type Throttle struct {
	// Used to group logs. Groups are rate limited independently (default: kubernetes.container_name)
	GroupKey string `json:"group_key,omitempty"`
	// This is the period of of time over which group_bucket_limit applies (default: 60)
	GroupBucketPeriodSeconds int `json:"group_bucket_period_s,omitempty"`
	// Maximum number logs allowed per groups over the period of group_bucket_period_s (default: 6000)
	GroupBucketLimit int `json:"group_bucket_limit,omitempty"`
	// When a group reaches its limit, logs will be dropped from further processing if this value is true (default: true)
	GroupDropLogs bool `json:"group_drop_logs,omitempty"`
	// After a group has exceeded its bucket limit, logs are dropped until the rate per second falls below or equal to group_reset_rate_s. (default: group_bucket_limit/group_bucket_period_s)
	GroupResetRateSeconds int `json:"group_reset_rate_s,omitempty"`
	// When a group reaches its limit and as long as it is not reset, a warning message with the current log rate of the group is emitted repeatedly. This is the delay between every repetition. (default: 10 seconds)
	GroupWarningDelaySeconds int `json:"group_warning_delay_s,omitempty"`
}

//
/*
## Example `Throttle` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - throttle:
        group_key: "$.kubernetes.container_name"
  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<filter **>
  @type throttle
  @id test_throttle
  group_key $.kubernetes.container_name
</filter>
{{</ highlight >}}
*/
type _expThrottle interface{} //nolint:deadcode,unused

func (t *Throttle) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "throttle"
	throttle := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	throttleConfig := t.DeepCopy()
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(throttleConfig); err != nil {
		return nil, err
	} else {
		throttle.Params = params
	}
	return throttle, nil
}

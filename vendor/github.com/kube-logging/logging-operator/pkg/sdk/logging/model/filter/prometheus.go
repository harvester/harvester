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

// +name:"Prometheus"
// +weight:"200"
type _hugoPrometheus interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Prometheus Filter](https://github.com/fluent/fluent-plugin-prometheus#prometheus-outputfilter-plugin)"
// Prometheus Filter Plugin to count Incoming Records
type _docPrometheus interface{} //nolint:deadcode,unused

// +name:"Prometheus"
// +url:"https://github.com/fluent/fluent-plugin-prometheus#prometheus-outputfilter-plugin"
// +version:"2.0.2"
// +description:"Prometheus Filter Plugin to count Incoming Records"
// +status:"GA"
type _metaPrometheus interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type PrometheusConfig struct {
	// +docLink:"Metrics Section,#metrics-section"
	Metrics []MetricSection `json:"metrics,omitempty"`
	Labels  Label           `json:"labels,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"Metrics Section"
type MetricSection struct {
	// Metrics name
	Name string `json:"name"`
	// Metrics type [counter](https://github.com/fluent/fluent-plugin-prometheus#counter-type), [gauge](https://github.com/fluent/fluent-plugin-prometheus#gauge-type), [summary](https://github.com/fluent/fluent-plugin-prometheus#summary-type), [histogram](https://github.com/fluent/fluent-plugin-prometheus#histogram-type)
	Type string `json:"type"`
	// Description of metric
	Desc string `json:"desc"`
	// Key name of record for instrumentation.
	Key string `json:"key,omitempty"`
	// Buckets of record for instrumentation
	Buckets string `json:"buckets,omitempty"`
	// Additional labels for this metric
	Labels Label `json:"labels,omitempty"`
}

//
/*
## Example `Prometheus` filter configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Flow
metadata:
  name: demo-flow
spec:
  filters:
    - tag_normaliser: {}
    - parser:
        remove_key_name_field: true
        reserve_data: true
        parse:
          type: nginx
    - prometheus:
        metrics:
        - name: total_counter
          desc: The total number of foo in message.
          type: counter
          labels:
            foo: bar
        labels:
          host: ${hostname}
          tag: ${tag}
          namespace: $.kubernetes.namespace
  selectors: {}
  localOutputRefs:
    - demo-output
{{</ highlight >}}

Fluentd config result:

{{< highlight xml>}}
  <filter **>
    @type prometheus
    @id logging-demo-flow_2_prometheus
    <metric>
      desc The total number of foo in message.
      name total_counter
      type counter
      <labels>
        foo bar
      </labels>
    </metric>
    <labels>
      host ${hostname}
      namespace $.kubernetes.namespace
      tag ${tag}
    </labels>
  </filter>
{{</ highlight >}}
*/
type _expPrometheus interface{} //nolint:deadcode,unused

type Label map[string]string

func (r Label) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	recordSet := types.PluginMeta{
		Directive: "labels",
	}
	directive := &types.GenericDirective{
		PluginMeta: recordSet,
		Params:     r,
	}
	return directive, nil
}

func (m *MetricSection) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	metricSection := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Directive: "metric",
		},
	}
	metric := m.DeepCopy()
	// Render Labels as subdirective
	if metric.Labels != nil {
		if format, err := metric.Labels.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			metricSection.SubDirectives = append(metricSection.SubDirectives, format)
		}
	}
	// Remove Labels from parameter rendering
	metric.Labels = nil
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(metric); err != nil {
		return nil, err
	} else {
		metricSection.Params = params
	}
	return metricSection, nil
}

func (p *PrometheusConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "prometheus"
	prometheus := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}

	prometheusConfig := p.DeepCopy()

	for _, metrics := range p.Metrics {
		if meta, err := metrics.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			prometheus.SubDirectives = append(prometheus.SubDirectives, meta)
		}
	}

	if p.Labels != nil {
		if format, err := p.Labels.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			prometheus.SubDirectives = append(prometheus.SubDirectives, format)
		}
	}

	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(prometheusConfig); err != nil {
		return nil, err
	} else {
		prometheus.Params = params
	}

	return prometheus, nil
}

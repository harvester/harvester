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
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
	util "github.com/banzaicloud/operator-tools/pkg/utils"
)

// +name:"Grafana Loki"
// +weight:"200"
type _hugoLoki interface{} //nolint:deadcode,unused

// +docName:"Loki output plugin "
// Fluentd output plugin to ship logs to a Loki server.
// More info at https://github.com/banzaicloud/fluent-plugin-kubernetes-loki
// >Example: [Store Nginx Access Logs in Grafana Loki with Logging Operator](../../../../quickstarts/loki-nginx/)
//
// ## Example output configurations
// ```yaml
// spec:
//
//	loki:
//	  url: http://loki:3100
//	  buffer:
//	    timekey: 1m
//	    timekey_wait: 30s
//	    timekey_use_utc: true
//
// ```
type _docLoki interface{} //nolint:deadcode,unused

// +name:"Grafana Loki"
// +url:"https://github.com/grafana/loki/tree/master/fluentd/fluent-plugin-grafana-loki"
// +version:"1.2.17"
// +description:"Transfer logs to Loki"
// +status:"GA"
type _metaLoki interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type LokiOutput struct {
	// The url of the Loki server to send logs to. (default:https://logs-us-west1.grafana.net)
	Url string `json:"url,omitempty"`
	// Specify a username if the Loki server requires authentication.
	// +docLink:"Secret,../secret/"
	Username *secret.Secret `json:"username,omitempty"`
	// Specify password if the Loki server requires authentication.
	// +docLink:"Secret,../secret/"
	Password *secret.Secret `json:"password,omitempty"`
	// TLS: parameters for presenting a client certificate
	// +docLink:"Secret,../secret/"
	Cert *secret.Secret `json:"cert,omitempty"`
	// TLS: parameters for presenting a client certificate
	// +docLink:"Secret,../secret/"
	Key *secret.Secret `json:"key,omitempty"`
	// TLS: CA certificate file for server certificate verification
	// +docLink:"Secret,../secret/"
	CaCert *secret.Secret `json:"ca_cert,omitempty"`
	// TLS: disable server certificate verification (default: false)
	InsecureTLS *bool `json:"insecure_tls,omitempty"`
	// Loki is a multi-tenant log storage platform and all requests sent must include a tenant.
	Tenant string `json:"tenant,omitempty"`
	// Set of labels to include with every Loki stream.
	Labels Label `json:"labels,omitempty"`
	// Set of extra labels to include with every Loki stream.
	ExtraLabels map[string]string `json:"extra_labels,omitempty"`
	// Format to use when flattening the record to a log line: json, key_value (default: key_value)
	LineFormat string `json:"line_format,omitempty" plugin:"default:json"`
	// Extract kubernetes labels as loki labels (default: false)
	ExtractKubernetesLabels *bool `json:"extract_kubernetes_labels,omitempty"`
	// Comma separated list of needless record keys to remove (default: [])
	RemoveKeys []string `json:"remove_keys,omitempty"`
	// If a record only has 1 key, then just set the log line to the value and discard the key. (default: false)
	DropSingleKey *bool `json:"drop_single_key,omitempty"`
	// Configure Kubernetes metadata in a Prometheus like format (default: false)
	ConfigureKubernetesLabels *bool `json:"configure_kubernetes_labels,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

type Label map[string]string

func (r Label) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	recordSet := types.PluginMeta{
		Directive: "label",
	}
	directive := &types.GenericDirective{
		PluginMeta: recordSet,
		Params:     r,
	}
	return directive, nil
}

func (r Label) merge(input Label) {
	for k, v := range input {
		r[k] = v
	}
}

func (l *LokiOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "loki"
	loki := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if l.ConfigureKubernetesLabels != nil && *l.ConfigureKubernetesLabels { // nolint:nestif
		if l.Labels == nil {
			l.Labels = Label{}
		}
		l.Labels.merge(Label{
			"namespace":    `$.kubernetes.namespace_name`,
			"pod":          `$.kubernetes.pod_name`,
			"container_id": `$.kubernetes.docker_id`,
			"container":    `$.kubernetes.container_name`,
			"pod_id":       `$.kubernetes.pod_id`,
			"host":         `$.kubernetes.host`,
		})

		if l.RemoveKeys != nil {
			if !util.Contains(l.RemoveKeys, "kubernetes") {
				l.RemoveKeys = append(l.RemoveKeys, "kubernetes")
			}
		} else {
			l.RemoveKeys = []string{"kubernetes"}
		}
		if l.ExtractKubernetesLabels == nil {
			l.ExtractKubernetesLabels = util.BoolPointer(true)
		}
		// Prevent meta configuration from marshalling
		l.ConfigureKubernetesLabels = nil
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(l); err != nil {
		return nil, err
	} else {
		loki.Params = params
	}
	if l.Labels != nil {
		if meta, err := l.Labels.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			loki.SubDirectives = append(loki.SubDirectives, meta)
		}
	}
	if l.Buffer == nil {
		l.Buffer = &Buffer{}
	}
	if buffer, err := l.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		loki.SubDirectives = append(loki.SubDirectives, buffer)
	}

	return loki, nil
}

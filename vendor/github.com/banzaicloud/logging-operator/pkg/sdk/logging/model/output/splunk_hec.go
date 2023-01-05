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
	"errors"

	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Splunk"
// +weight:"200"
type _hugoSplunk interface{} //nolint:deadcode,unused

// +docName:"Splunk via Hec output plugin for Fluentd"
// More info at https://github.com/splunk/fluent-plugin-splunk-hec
//
// ## Example output configurations
// ```yaml
// spec:
//
//	splunkHec:
//	  hec_host: splunk.default.svc.cluster.local
//	  hec_port: 8088
//	  protocol: http
//
// ```
type _docSplunkHec interface{} //nolint:deadcode,unused

// +name:"Splunk Hec"
// +url:"https://github.com/splunk/fluent-plugin-splunk-hec/releases/tag/1.2.9
// +version:"1.2.9"
// +description:"Fluent Plugin Splunk Hec Release"
// +status:"GA"
type _metaSplunkHec interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"SplunkHecOutput"
// SplunkHecOutput sends your logs to Splunk via Hec
type SplunkHecOutput struct {
	// The type of data that will be sent to Sumo Logic, either event or metric (default: event)
	DataType string `json:"data_type,omitempty"`
	// You can specify SplunkHec host by this parameter.
	HecHost string `json:"hec_host"`
	// The port number for the Hec token or the Hec load balancer. (default: 8088)
	HecPort int `json:"hec_port,omitempty"`
	// This is the protocol to use for calling the Hec API. Available values are: http, https. (default: https)
	Protocol string `json:"protocol,omitempty"`
	// Identifier for the Hec token.
	// +docLink:"Secret,../secret/"
	HecToken *secret.Secret `json:"hec_token"`
	// When data_type is set to "metric", the ingest API will treat every key-value pair in the input event as a metric name-value pair. Set metrics_from_event to false to disable this behavior and use metric_name_key and metric_value_key to define metrics. (Default:true)
	MetricsFromEvent *bool `json:"metrics_from_event,omitempty"`
	// Field name that contains the metric name. This parameter only works in conjunction with the metrics_from_event parameter. When this prameter is set, the metrics_from_event parameter is automatically set to false. (default: true)
	MetricNameKey string `json:"metric_name_key,omitempty"`
	// Field name that contains the metric value, this parameter is required when metric_name_key is configured.
	MetricValueKey string `json:"metric_value_key,omitempty"`
	// Indicates whether to allow non-UTF-8 characters in user logs. If set to true, any non-UTF-8 character is replaced by the string specified in non_utf8_replacement_string. If set to false, the Ingest API errors out any non-UTF-8 characters. (default: true).
	CoerceToUtf8 *bool `json:"coerce_to_utf8,omitempty"`
	// If coerce_to_utf8 is set to true, any non-UTF-8 character is replaced by the string you specify in this parameter. (default: ' ').
	NonUtf8ReplacementString string `json:"non_utf8_replacement_string,omitempty"`
	// Identifier for the Splunk index to be used for indexing events. If this parameter is not set, the indexer is chosen by HEC. Cannot set both index and index_key parameters at the same time.
	Index string `json:"index,omitempty"`
	// The field name that contains the Splunk index name. Cannot set both index and index_key parameters at the same time.
	IndexKey string `json:"index_key,omitempty"`
	// The host location for events. Cannot set both host and host_key parameters at the same time. (Default:hostname)
	Host string `json:"host,omitempty"`
	// Key for the host location. Cannot set both host and host_key parameters at the same time.
	HostKey string `json:"host_key,omitempty"`
	// The source field for events. If this parameter is not set, the source will be decided by HEC. Cannot set both source and source_key parameters at the same time.
	Source string `json:"source,omitempty"`
	// Field name to contain source. Cannot set both source and source_key parameters at the same time.
	SourceKey string `json:"source_key,omitempty"`
	// The sourcetype field for events. When not set, the sourcetype is decided by HEC. Cannot set both source and source_key parameters at the same time.
	SourceType string `json:"sourcetype,omitempty"`
	// Field name that contains the sourcetype. Cannot set both source and source_key parameters at the same time.
	SourceTypeKey string `json:"sourcetype_key,omitempty"`
	// By default, all the fields used by the *_key parameters are removed from the original input events. To change this behavior, set this parameter to true. This parameter is set to false by default. When set to true, all fields defined in index_key, host_key, source_key, sourcetype_key, metric_name_key, and metric_value_key are saved in the original event.
	KeepKeys bool `json:"keep_keys,omitempty"`
	//If a connection has not been used for this number of seconds it will automatically be reset upon the next use to avoid attempting to send to a closed connection. nil means no timeout.
	IdleTimeout int `json:"idle_timeout,omitempty"`
	// The amount of time allowed between reading two chunks from the socket.
	ReadTimeout int `json:"read_timeout,omitempty"`
	// The amount of time to wait for a connection to be opened.
	OpenTimeout int `json:"open_timeout,omitempty"`
	// The path to a file containing a PEM-format CA certificate for this client.
	// +docLink:"Secret,../secret/"
	ClientCert *secret.Secret `json:"client_cert,omitempty"`
	// The private key for this client.'
	// +docLink:"Secret,../secret/"
	ClientKey *secret.Secret `json:"client_key,omitempty"`
	// The path to a file containing a PEM-format CA certificate.
	// +docLink:"Secret,../secret/"
	CAFile *secret.Secret `json:"ca_file,omitempty"`
	// The path to a directory containing CA certificates in PEM format.
	// +docLink:"Secret,../secret/"
	CAPath *secret.Secret `json:"ca_path,omitempty"`
	// List of SSL ciphers allowed.
	SSLCiphers string `json:"ssl_ciphers,omitempty"`
	// Indicates if insecure SSL connection is allowed (default:false)
	InsecureSSL *bool `json:"insecure_ssl,omitempty"`
	// In this case, parameters inside <fields> are used as indexed fields and removed from the original input events
	Fields Fields `json:"fields,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

type Fields map[string]string

func (r Fields) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	recordSet := types.PluginMeta{
		Directive: "fields",
	}
	directive := &types.GenericDirective{
		PluginMeta: recordSet,
		Params:     r,
	}
	return directive, nil
}

func (c *SplunkHecOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "splunk_hec"
	splunkHec := &types.OutputPlugin{
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
		splunkHec.Params = params
	}

	if err := c.validateConflictingFields(); err != nil {
		return nil, err
	}
	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		splunkHec.SubDirectives = append(splunkHec.SubDirectives, buffer)
	}

	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			splunkHec.SubDirectives = append(splunkHec.SubDirectives, format)
		}
	}

	if c.Fields != nil {
		if meta, err := c.Fields.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			splunkHec.SubDirectives = append(splunkHec.SubDirectives, meta)
		}
	}

	return splunkHec, nil
}

func (c *SplunkHecOutput) validateConflictingFields() error {
	if c.Index != "" && c.IndexKey != "" {
		return errors.New("index and index_key cannot be set simultaneously")
	}
	if c.Host != "" && c.HostKey != "" {
		return errors.New("host and host_key cannot be set simultaneously")
	}
	if c.Source != "" && c.SourceKey != "" {
		return errors.New("source and source_key cannot be set simultaneously")
	}
	if c.SourceType != "" && c.SourceTypeKey != "" {
		return errors.New("sourcetype and sourcetype_key cannot be set simultaneously")
	}

	return nil
}

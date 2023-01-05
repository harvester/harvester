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
)

// +name:"Datadog"
// +weight:"200"
type _hugoDatadog interface{} //nolint:deadcode,unused

// +docName:"Datadog output plugin for Fluentd"
// It mainly contains a proper JSON formatter and a socket handler that streams logs directly to Datadog - so no need to use a log shipper if you don't wan't to.
// More info at [https://github.com/DataDog/fluent-plugin-datadog](https://github.com/DataDog/fluent-plugin-datadog).
//
// ## Example
// ```yaml
// spec:
//
//	datadog:
//	  api_key '<YOUR_API_KEY>'
//	  dd_source: '<INTEGRATION_NAME>'
//	  dd_tags: '<KEY1:VALUE1>,<KEY2:VALUE2>'
//	  dd_sourcecategory: '<YOUR_SOURCE_CATEGORY>'
//
// ```
type _docDatadog interface{} //nolint:deadcode,unused

// +name:"Datadog"
// +url:"https://github.com/DataDog/fluent-plugin-datadog/releases/tag/v0.14.1"
// +version:"0.14.1"
// +description:"Send your logs to Datadog"
// +status:"Testing"
type _metaDatadog interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type DatadogOutput struct {
	// This parameter is required in order to authenticate your fluent agent. (default: nil)
	// +docLink:"Secret,../secret/"
	ApiKey *secret.Secret `json:"api_key"`
	// Event format, if true, the event is sent in json format. Othwerwise, in plain text.  (default: true)
	UseJson bool `json:"use_json,omitempty"`
	// Automatically include the Fluentd tag in the record.  (default: false)
	IncludeTagKey bool `json:"include_tag_key,omitempty"`
	// Where to store the Fluentd tag. (default: "tag")
	TagKey string `json:"tag_key,omitempty"`
	//Name of the attribute which will contain timestamp of the log event. If nil, timestamp attribute is not added. (default: "@timestamp")
	TimestampKey string `json:"timestamp_key,omitempty"`
	// If true, the agent initializes a secure connection to Datadog. In clear TCP otherwise.  (default: true)
	UseSsl bool `json:"use_ssl,omitempty"`
	// Disable SSL validation (useful for proxy forwarding)  (default: false)
	NoSslValidation bool `json:"no_ssl_validation,omitempty"`
	// Port used to send logs over a SSL encrypted connection to Datadog. If use_http is disabled, use 10516 for the US region and 443 for the EU region. (default: "443")
	SslPort string `json:"ssl_port,omitempty"`
	// The number of retries before the output plugin stops. Set to -1 for unlimited retries (default: "-1")
	MaxRetries string `json:"max_retries,omitempty"`
	// The maximum time waited between each retry in seconds (default: "30")
	MaxBackoff string `json:"max_backoff,omitempty"`
	// Enable HTTP forwarding. If you disable it, make sure to change the port to 10514 or ssl_port to 10516  (default: true)
	UseHttp bool `json:"use_http,omitempty"`
	// Enable log compression for HTTP  (default: true)
	UseCompression bool `json:"use_compression,omitempty"`
	// Set the log compression level for HTTP (1 to 9, 9 being the best ratio) (default: "6")
	CompressionLevel string `json:"compression_level,omitempty"`
	// This tells Datadog what integration it is (default: nil)
	DdSource string `json:"dd_source,omitempty"`
	// Multiple value attribute. Can be used to refine the source attribute (default: nil)
	DdSourcecategory string `json:"dd_sourcecategory,omitempty"`
	// Custom tags with the following format "key1:value1, key2:value2" (default: nil)
	DdTags string `json:"dd_tags,omitempty"`
	// Used by Datadog to identify the host submitting the logs. (default: "hostname -f")
	DdHostname string `json:"dd_hostname,omitempty"`
	// Used by Datadog to correlate between logs, traces and metrics. (default: nil)
	Service string `json:"service,omitempty"`
	// Proxy port when logs are not directly forwarded to Datadog and ssl is not used (default: "80")
	Port string `json:"port,omitempty"`
	// Proxy endpoint when logs are not directly forwarded to Datadog	 (default: "http-intake.logs.datadoghq.com")
	Host string `json:"host,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (a *DatadogOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "datadog"
	datadog := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(a); err != nil {
		return nil, err
	} else {
		datadog.Params = params
	}
	if a.Buffer == nil {
		a.Buffer = &Buffer{}
	}
	if buffer, err := a.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		datadog.SubDirectives = append(datadog.SubDirectives, buffer)
	}
	return datadog, nil
}

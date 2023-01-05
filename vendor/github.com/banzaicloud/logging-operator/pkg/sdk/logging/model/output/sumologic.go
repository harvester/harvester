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

// +name:"SumoLogic"
// +weight:"200"
type _hugoSumoLogic interface{} //nolint:deadcode,unused

// +docName:"SumoLogic output plugin for Fluentd"
// This plugin has been designed to output logs or metrics to SumoLogic via a HTTP collector endpoint
// More info at https://github.com/SumoLogic/fluentd-output-sumologic
//
// Example secret for HTTP input URL
// ```
// kubectl create secret generic sumo-output --from-literal "endpoint=$URL"
// ```
//
// # Example ClusterOutput
//
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: ClusterOutput
// metadata:
//
//	name: sumo-output
//
// spec:
//
//	sumologic:
//	  buffer:
//	    flush_interval: 10s
//	    flush_mode: interval
//	  compress: true
//	  endpoint:
//	    valueFrom:
//	      secretKeyRef:
//	        key: endpoint
//	        name: sumo-output
//	  source_name: test1
//
// ```
//
//export URL='https://endpoint1.collection.eu.sumologic.com/receiver/v1/http/.......'
type _docSumoLogic interface{} //nolint:deadcode,unused

// +name:"SumoLogic"
// +url:"https://github.com/SumoLogic/fluentd-output-sumologic/releases/tag/1.8.0"
// +version:"1.8.0"
// +description:"Send your logs to Sumologic"
// +status:"GA"
type _metaSumologic interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type SumologicOutput struct {
	// The type of data that will be sent to Sumo Logic, either logs or metrics (default: logs)
	DataType string `json:"data_type,omitempty"`
	// SumoLogic HTTP Collector URL
	Endpoint *secret.Secret `json:"endpoint"`
	// Verify ssl certificate. (default: true)
	VerifySsl bool `json:"verify_ssl,omitempty"`
	// The format of metrics you will be sending, either graphite or carbon2 or prometheus (default: graphite)
	MetricDataFormat string `json:"metric_data_format,omitempty"`
	// Format to post logs into Sumo. (default: json)
	LogFormat string `json:"log_format,omitempty"`
	// Used to specify the key when merging json or sending logs in text format (default: message)
	LogKey string `json:"log_key,omitempty"`
	// Set _sourceCategory metadata field within SumoLogic (default: nil)
	SourceCategory string `json:"source_category,omitempty"`
	// Set _sourceName metadata field within SumoLogic - overrides source_name_key (default is nil)
	SourceName string `json:"source_name"`
	// Set as source::path_key's value so that the source_name can be extracted from Fluentd's buffer (default: source_name)
	SourceNameKey string `json:"source_name_key,omitempty"`
	// Set _sourceHost metadata field within SumoLogic (default: nil)
	SourceHost string `json:"source_host,omitempty"`
	// Set timeout seconds to wait until connection is opened. (default: 60)
	OpenTimeout int `json:"open_timeout,omitempty"`
	// Add timestamp (or timestamp_key) field to logs before sending to sumologic (default: true)
	AddTimestamp bool `json:"add_timestamp,omitempty"`
	// Field name when add_timestamp is on (default: timestamp)
	TimestampKey string `json:"timestamp_key,omitempty"`
	// Add the uri of the proxy environment if present.
	ProxyUri string `json:"proxy_uri,omitempty"`
	// Option to disable cookies on the HTTP Client. (default: false)
	DisableCookies bool `json:"disable_cookies,omitempty"`
	// Delimiter (default: .)
	Delimiter string `json:"delimiter,omitempty"`
	// Comma-separated key=value list of fields to apply to every log. [more information](https://help.sumologic.com/Manage/Fields#http-source-fields)
	CustomFields []string `json:"custom_fields,omitempty"`
	// Name of sumo client which is send as X-Sumo-Client header (default: fluentd-output)
	SumoClient string `json:"sumo_client,omitempty"`
	// Compress payload (default: false)
	Compress *bool `json:"compress,omitempty"`
	// Encoding method of compression (either gzip or deflate) (default: gzip)
	CompressEncoding string `json:"compress_encoding,omitempty"`
	// Dimensions string (eg "cluster=payment, service=credit_card") which is going to be added to every metric record.
	CustomDimensions string `json:"custom_dimensions,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (s *SumologicOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "sumologic"
	sumologic := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(s); err != nil {
		return nil, err
	} else {
		sumologic.Params = params
	}
	if s.Buffer == nil {
		s.Buffer = &Buffer{}
	}
	if buffer, err := s.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		sumologic.SubDirectives = append(sumologic.SubDirectives, buffer)
	}
	return sumologic, nil
}

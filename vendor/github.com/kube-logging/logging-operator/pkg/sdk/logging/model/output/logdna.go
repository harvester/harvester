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

// +name:"LogDNA"
// +weight:"200"
type _hugoLogDNA interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[LogDNA Output](https://github.com/logdna/fluent-plugin-logdna)"
// This plugin has been designed to output logs to LogDNA.
type _docLogDNA interface{} //nolint:deadcode,unused

// +name:"LogDNA"
// +url:"https://github.com/logdna/fluent-plugin-logdna/releases/tag/v0.4.0"
// +version:"0.4.0"
// +description:"Send your logs to LogDNA"
// +status:"GA"
type _metaLogDNA interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"LogDNA"
// Send your logs to LogDNA
type LogDNAOutput struct {
	// LogDNA Api key
	ApiKey string `json:"api_key"`
	// Hostname
	HostName string `json:"hostname"`
	// Application name
	App string `json:"app,omitempty"`
	// Comma-Separated List of Tags, Optional
	Tags string `json:"tags,omitempty"`
	// HTTPS POST Request Timeout, Optional. Supports s and ms Suffices (default: 30 s)
	RequestTimeout string `json:"request_timeout,omitempty"`
	// Custom Ingester URL, Optional (default: https://logs.logdna.com)
	IngesterDomain string `json:"ingester_domain,omitempty"`
	// Custom Ingester Endpoint, Optional (default: /logs/ingest)
	IngesterEndpoint string `json:"ingester_endpoint,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

// ## Example `LogDNA` filter configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Output
// metadata:
//
//	name: logdna-output-sample
//
// spec:
//
//	logdna:
//	  api_key: xxxxxxxxxxxxxxxxxxxxxxxxxxx
//	  hostname: logging-operator
//	  app: my-app
//	  tags: web,dev
//	  ingester_domain https://logs.logdna.com
//	  ingester_endpoint /logs/ingest
//
// ```
//
// #### Fluentd Config Result
// ```
// <match **>
//
//	@type logdna
//	@id test_logdna
//	api_key xxxxxxxxxxxxxxxxxxxxxxxxxxy
//	app my-app
//	hostname logging-operator
//
// </match>
// ```
type _expLogDNA interface{} //nolint:deadcode,unused

func (l *LogDNAOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "logdna"
	pluginID := id + "_" + pluginType
	logdna := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        pluginID,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(l); err != nil {
		return nil, err
	} else {
		logdna.Params = params
	}
	if l.Buffer == nil {
		l.Buffer = &Buffer{}
	}
	if buffer, err := l.Buffer.ToDirective(secretLoader, pluginID); err != nil {
		return nil, err
	} else {
		logdna.SubDirectives = append(logdna.SubDirectives, buffer)
	}

	return logdna, nil
}

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

// +name:"Format rfc5424"
// +weight:"200"
type _hugoFormatRfc5424 interface{} //nolint:deadcode,unused

// +name:"Format rfc5424"
// +url:"https://github.com/cloudfoundry/fluent-plugin-syslog_rfc5424#format-section"
// +version:"more info"
// +description:"Specify how to format output record."
// +status:"GA"
type _metaFormatRfc5424 interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type FormatRfc5424 struct {
	// Output line formatting: out_file,json,ltsv,csv,msgpack,hash,single_value (default: json)
	// +kubebuilder:validation:Enum=out_file;json;ltsv;csv;msgpack;hash;single_value
	Type string `json:"type,omitempty"`
	// Prepends message length for syslog transmission (default: true)
	Rfc6587MessageSize *bool `json:"rfc6587_message_size,omitempty"`
	// Sets host name in syslog from field in fluentd, delimited by '.' (default: hostname)
	HostnameField string `json:"hostname_field,omitempty"`
	// Sets app name in syslog from field in fluentd, delimited by '.' (default: app_name)
	AppNameField string `json:"app_name_field,omitempty"`
	// Sets proc id in syslog from field in fluentd, delimited by '.'  (default: proc_id)
	ProcIdField string `json:"proc_id_field,omitempty"`
	// Sets msg id in syslog from field in fluentd, delimited by '.' (default: message_id)
	MessageIdField string `json:"message_id_field,omitempty"`
	// Sets structured data in syslog from field in fluentd, delimited by '.' (default structured_data)
	StructuredDataField string `json:"structured_data_field,omitempty"`
	// Sets log in syslog from field in fluentd, delimited by '.' (default: log)
	LogField string `json:"log_field,omitempty"`
}

func (f *FormatRfc5424) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	metadata := types.PluginMeta{
		Directive: "format",
	}
	format := f.DeepCopy()
	if format.Type != "" {
		metadata.Type = format.Type
	} else {
		metadata.Type = "syslog_rfc5424"
	}
	format.Type = ""
	return types.NewFlatDirective(metadata, format, secretLoader)
}

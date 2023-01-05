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

// +name:"Format"
// +weight:"200"
type _hugoFormat interface{} //nolint:deadcode,unused

// +docName:"Format output records"
// Specify how to format output records. For details, see [https://docs.fluentd.org/configuration/format-section](https://docs.fluentd.org/configuration/format-section).
//
// ## Example
// ```yaml
// spec:
//
//	format:
//	  path: /tmp/logs/${tag}/%Y/%m/%d.%H.%M
//	  format:
//	    type: single_value
//	    add_newline: true
//	    message_key: msg
//
// ```
type _docFormat interface{} //nolint:deadcode,unused

// +name:"Format"
// +url:"https://docs.fluentd.org/configuration/format-section"
// +version:"more info"
// +description:"Specify how to format output record."
// +status:"GA"
type _metaFormat interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type Format struct {
	// Output line formatting: out_file,json,ltsv,csv,msgpack,hash,single_value (default: json)
	// +kubebuilder:validation:Enum=out_file;json;ltsv;csv;msgpack;hash;single_value
	Type string `json:"type,omitempty"`
	// When type is single_value add '\n' to the end of the message (default: true)
	AddNewline *bool `json:"add_newline,omitempty"`
	// When type is single_value specify the key holding information
	MessageKey string `json:"message_key,omitempty"`
}

func (f *Format) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	metadata := types.PluginMeta{
		Directive: "format",
	}
	format := f.DeepCopy()
	if format.Type != "" {
		metadata.Type = format.Type
	} else {
		metadata.Type = "json"
	}
	format.Type = ""
	return types.NewFlatDirective(metadata, format, secretLoader)
}

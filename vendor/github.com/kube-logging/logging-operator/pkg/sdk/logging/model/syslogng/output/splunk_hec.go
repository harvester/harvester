// Copyright © 2023 Kube logging authors
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

import "github.com/cisco-open/operator-tools/pkg/secret"

// +name:"SplunkHEC"
// +weight:"200"
type _hugoSplunkHEC interface{} //nolint:deadcode,unused

// +docName:"Sending messages over Splunk HEC"
/*
Based on the [Splunk destination of AxoSyslog core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/syslog-ng-with-splunk/).

Available in Logging operator version 4.4 and later.

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: splunkhec
spec:
  splunk_hec_event:
    url: "https://splunk-endpoint"
    token:
      valueFrom:
          secretKeyRef:
            name: splunk-hec
            key: token
{{</ highlight >}}
*/
type _docSplunkHEC interface{} //nolint:deadcode,unused

// +name:"SplunkHEC"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/syslog-ng-with-splunk/
// +description:"Sending messages over Splunk HEC"
// +status:"Testing"
type _metaSplunkHEC interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type SplunkHECOutput struct {
	HTTPOutput `json:",inline"`
	// The token that syslog-ng OSE uses to authenticate on the event collector.
	Token secret.Secret `json:"token,omitempty"`
	// event() accepts a template, which declares the content of the log message sent to Splunk. Default value: `${MSG}`
	Event string `json:"event,omitempty"`
	// Splunk index where the messages will be stored.
	Index string `json:"index,omitempty"`
	// Sets the source field.
	Source string `json:"source,omitempty"`
	// Sets the sourcetype field.
	Sourcetype string `json:"sourcetype,omitempty"`
	// Sets the host field.
	Host string `json:"host,omitempty"`
	// Sets the time field.
	Time string `json:"time,omitempty"`
	// Fallback option for index field.
	// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/syslog-ng-with-splunk/).
	DefaultIndex string `json:"default_index,omitempty"`
	// Fallback option for source field.
	DefaultSource string `json:"default_source,omitempty"`
	// Fallback option for sourcetype field.
	DefaultSourcetype string `json:"default_sourcetype,omitempty"`
	// Additional indexing metadata for Splunk.
	Fields string `json:"fields,omitempty"`
	// Additional HTTP request headers.
	ExtraHeaders []string `json:"extra_headers,omitempty"`
	// Additional HTTP request query options.
	ExtraQueries []string `json:"extra_queries,omitempty"`
	// Additional HTTP request content-type option.
	ContentType string `json:"content_type,omitempty"`
}

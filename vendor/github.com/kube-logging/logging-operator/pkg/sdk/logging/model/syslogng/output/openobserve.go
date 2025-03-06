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

// +name:"OpenObserve"
// +weight:"200"
type _hugoOpenobserve interface{} //nolint:deadcode,unused

// +docName:"Sending messages over OpenObserve"
/*
Send messages to [OpenObserve](https://openobserve.ai/docs/api/ingestion/logs/json/) using its [Logs Ingestion - JSON API](https://openobserve.ai/docs/api/ingestion/logs/json/). This API accepts multiple records in batch in JSON format.

Available in Logging operator version 4.5 and later.

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: openobserve
spec:
  openobserve:
    url: "https://some-openobserve-endpoint"
    port: 5080
    organization: "default"
    stream: "default"
    user: "username"
    password:
      valueFrom:
        secretKeyRef:
          name: openobserve
          key: password
{{</ highlight >}}

For details on the available options of the output, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/openobserve/).
*/
type _docOpenobserve interface{} //nolint:deadcode,unused

// +name:"OpenObserve"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/openobserve/"
// +description:"Sending messages over OpenObserve"
// +status:"Testing"
type _metaOpenobserve interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type OpenobserveOutput struct {
	HTTPOutput `json:",inline"`
	// The port number of the OpenObserve server. (default: 5080)
	// Specify it here instead of appending it to the URL.
	Port int `json:"port,omitempty"`
	// Name of the organization in OpenObserve.
	Organization string `json:"organization,omitempty"`
	// Name of the stream in OpenObserve.
	Stream string `json:"stream,omitempty"`
	// Arguments to the `$format-json()` template function.
	// Default: `"--scope rfc5424 --exclude DATE --key ISODATE @timestamp=${ISODATE}"`
	Record string `json:"record,omitempty"`
}

func (o *OpenobserveOutput) BeforeRender() {
	if o.Port == 0 {
		o.Port = 5080
	}

	if o.Organization == "" {
		o.Organization = "default"
	}

	if o.Stream == "" {
		o.Stream = "default"
	}

}

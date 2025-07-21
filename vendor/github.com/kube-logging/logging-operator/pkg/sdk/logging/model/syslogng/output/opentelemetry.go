// Copyright © 2024 Kube logging authors
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

import "github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"

// +name:"OpenTelemetry output"
// +weight:"200"
type _hugoOpenTelemetry interface{} //nolint:deadcode,unused

// +docName:"Sending fluent structured messages over OpenTelemetry GRPC"
/*
Sends messages over OpenTelemetry GRPC. For details on the available options of the output, see the [documentation of AxoSyslog](https://axoflow.com/docs/axosyslog-core/chapter-destinations/opentelemetry/).

## Example

A simple example sending logs over OpenTelemetry GRPC to a remote OpenTelemetry endpoint:

{{< highlight yaml >}}
kind: SyslogNGOutput
apiVersion: logging.banzaicloud.io/v1beta1
metadata:
  name: otlp
spec:
  opentelemetry:
    url: otel-server
    port: 4379
{{</ highlight >}}

*/
type _docOpenTelemetry interface{} //nolint:deadcode,unused

// +name:"OpenTelemetry"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/opentelemetry/"
// +description:"Sending messages over OpenTelemetry GRPC"
// +status:"Testing"
type _metaOTLP interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type OpenTelemetryOutput struct {
	// Specifies the hostname or IP address and optionally the port number of the web service that can receive log data via HTTP. Use a colon (:) after the address to specify the port number of the server. For example: `http://127.0.0.1:8000`
	URL string `json:"url"`
	// Authentication configuration, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-syslog-ng-otlp/#auth).
	Auth *Auth `json:"auth,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Batching parameters
	Batch `json:",inline"`
	// Enable or disable compression. (default: false)
	Compression *bool `json:"compression,omitempty"`
	// Add GRPC Channel arguments https://axoflow.com/docs/axosyslog-core/chapter-destinations/opentelemetry/#channel-args
	ChannelArgs filter.ArrowMap `json:"channel_args,omitempty"`
}

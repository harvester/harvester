// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

// +name:"LogScale"
// +weight:"200"
type _hugoLogScale interface{} //nolint:deadcode,unused

// +docName:"Storing messages in Falcon LogScale"
// The `LogScale` output sends log records over HTTP to Falcon's LogScale.
//
/*
Based on the [LogScale destination of AxoSyslog core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/crowdstrike-falcon/). Sends log records over HTTP to Falcon's LogScale.

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: test-logscale
  namespace: logging
spec:
  logscale:
    token:
      valueFrom:
        secretKeyRef:
          key: token
          name: logscale-token
    timezone: "UTC"
    batch_lines: 1000
    disk_buffer:
      disk_buf_size: 512000000
      dir: /buffers
      reliable: true
{{</ highlight >}}
*/
type _docLogScale interface{} //nolint:deadcode,unused

// +name:"Falcon LogScale"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/crowdstrike-falcon/"
// +description:"Storing messages in Falcon's LogScale over http"
// +status:"Testing"
type _metaLogScale interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type LogScaleOutput struct {
	// Ingester URL is the URL of the Humio cluster you want to send data to. (default: https://cloud.humio.com)
	URL *secret.Secret `json:"url,omitempty"`
	// An [Ingest Token](https://library.humio.com/data-analysis/ingesting-data-tokens.html) is a unique string that identifies a repository and allows you to send data to that repository. (default: empty)
	Token *secret.Secret `json:"token,omitempty"`
	// The raw string representing the Event. The default display for an Event in LogScale is the rawstring. If you do not provide the rawstring field, then the response defaults to a JSON representation of the attributes field. (default: empty)
	RawString string `json:"rawstring,omitempty"`
	// 	A JSON object representing key-value pairs for the Event. These key-value pairs adds structure to Events, making it easier to search. Attributes can be nested JSON objects, however, we recommend limiting the amount of nesting. (default: `"--scope rfc5424 --exclude MESSAGE --exclude DATE --leave-initial-dot"`)
	Attributes string `json:"attributes,omitempty"`
	// The timezone is only required if you specify the timestamp in milliseconds. The timezone specifies the local timezone for the event. Note that you must still specify the timestamp in UTC time.
	TimeZone string `json:"timezone,omitempty"`
	//  This field represents additional headers that can be included in the HTTP request when sending log records to Falcon's LogScale.  (default: empty)
	ExtraHeaders string `json:"extra_headers,omitempty"`
	//   This field specifies the content type of the log records being sent to Falcon's LogScale. (default: `"application/json"`)
	ContentType string `json:"content_type,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer   *DiskBuffer `json:"disk_buffer,omitempty"`
	Body         string      `json:"body,omitempty"`
	BatchLines   int         `json:"batch_lines,omitempty"`
	BatchBytes   int         `json:"batch_bytes,omitempty"`
	BatchTimeout int         `json:"batch_timeout,omitempty"`
	PersistName  string      `json:"persist_name,omitempty"`
}

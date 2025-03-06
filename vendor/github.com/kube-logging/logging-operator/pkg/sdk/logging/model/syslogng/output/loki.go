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

import (
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
)

// +name:"Loki"
// +weight:"200"
type _hugoLoki interface{} //nolint:deadcode,unused

// +docName:"Sending messages to Loki over gRPC"
/*
Sends messages to Grafana Loki over gRPC, based on the [Loki destination of AxoSyslog Core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-loki/).

Available in Logging operator version 4.4 and later.

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: loki-output
spec:
  loki:
    url: "loki.loki:8000"
    batch-lines: 2000
    batch-timeout: 10
    workers: 3
    log-fifo-size: 1000
    labels:
      "app": "$PROGRAM"
      "host": "$HOST"
    timestamp: "msg"
    template: "$ISODATE $HOST $MSGHDR$MSG"
    auth:
      insecure: {}
{{</ highlight >}}

For details on the available options of the output, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-loki/). For available macros like `$PROGRAM` and `$HOST` see https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/customizing-message-format/reference-macros/
*/
type _docLoki interface{} //nolint:deadcode,unused

// +name:"Loki"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-loki/"
// +description:"Sending messages to Loki over gRPC"
// +status:"Testing"
type _metaLoki interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type LokiOutput struct {
	// Authentication configuration, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-loki/#auth).
	Auth *Auth `json:"auth,omitempty"`
	// Using the Labels map, Kubernetes label to Loki label mapping can be configured. Example: `{"app" : "$PROGRAM"}`
	Labels filter.ArrowMap `json:"labels,omitempty"`
	// Specifies the hostname or IP address and optionally the port number of the  service that can receive log data via gRPC. Use a colon (:) after the address to specify the port number of the server. For example: `grpc://127.0.0.1:8000`
	URL string `json:"url,omitempty"`
	// The time to wait in seconds before a dead connection is reestablished. (default: 60)
	TimeReopen int `json:"time_reopen,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Description: Specifies how many lines are flushed to a destination in one batch. The syslog-ng OSE application waits for this number of lines to accumulate and sends them off in a single batch. Increasing this number increases throughput as more messages are sent in a single batch, but also increases message latency.
	// For example, if you set batch-lines() to 100, syslog-ng OSE waits for 100 messages.
	BatchLines int `json:"batch-lines,omitempty"`
	// Description: Specifies the time syslog-ng OSE waits for lines to accumulate in the output buffer. The syslog-ng OSE application sends batches to the destinations evenly. The timer starts when the first message arrives to the buffer, so if only few messages arrive, syslog-ng OSE sends messages to the destination at most once every batch-timeout() milliseconds.
	BatchTimeout int `json:"batch-timeout,omitempty"`
	// The number of times syslog-ng OSE attempts to send a message to this destination. If syslog-ng OSE could not send a message, it will try again until the number of attempts reaches retries, then drops the message.
	Retries int `json:"retries,omitempty"`
	// Specifies the number of worker threads (at least 1) that syslog-ng OSE uses to send messages to the server. Increasing the number of worker threads can drastically improve the performance of the destination.
	Workers int `json:"workers,omitempty"`
	// If you receive the following error message during AxoSyslog startup, set the persist-name() option of the duplicate drivers:
	// `Error checking the uniqueness of the persist names, please override it with persist-name option. Shutting down.`
	// See [syslog-ng docs](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/reference-destination-http-nonjava/#persist-name) for more information.
	PersistName string `json:"persist_name,omitempty"`
	// The number of messages that the output queue can store.
	LogFIFOSize int `json:"log-fifo-size,omitempty"`
	// The timestamp that will be applied to the outgoing messages (possible values: current|received|msg default: current). Loki does not accept events, in which the timestamp is not monotonically increasing.
	// +kubebuilder:validation:Enum=current;received;msg
	Timestamp string `json:"timestamp,omitempty"`
	// Template for customizing the log message format.
	Template string `json:"template,omitempty"`
}

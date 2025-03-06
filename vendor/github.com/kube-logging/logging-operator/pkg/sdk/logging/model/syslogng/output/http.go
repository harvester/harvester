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

import (
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
)

// +name:"HTTP"
// +weight:"200"
type _hugoHTTP interface{} //nolint:deadcode,unused

// +docName:"Sending messages over HTTP"
/*
Sends messages over HTTP. For details on the available options of the output, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/).

## Example

A simple example sending logs over HTTP to a fluentbit HTTP endpoint:

{{< highlight yaml >}}
kind: SyslogNGOutput
apiVersion: logging.banzaicloud.io/v1beta1
metadata:
  name: http
spec:
  http:
    #URL of the ingest endpoint
    url: http://fluentbit-endpoint:8080/tag
    method: POST
    headers:
      - "Content-type: application/json"
{{</ highlight >}}

A more complex example to demonstrate sending logs to OpenObserve
{{< highlight yaml >}}
kind: SyslogNGOutput
apiVersion: logging.banzaicloud.io/v1beta1
metadata:
  name: openobserve
spec:
  http:
    #URL of the ingest endpoint
    url: https://openobserve-endpoint/api/default/log-generator/_json
    user: "username"
    password:
      valueFrom:
        secretKeyRef:
          name: openobserve
          key: password
    method: POST
    # Parameters for sending logs in batches
    batch-lines: 5000
    batch-bytes: 4096
    batch-timeout: 300
    headers:
      - "Connection: keep-alive"
    # Disable TLS peer verification for demo
    tls:
      peer_verify: "no"
    body-prefix: "["
    body-suffix: "]"
    delimiter: ","
    body: "${MESSAGE}"
{{</ highlight >}}

For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/).
*/
type _docHTTP interface{} //nolint:deadcode,unused

// +name:"HTTP"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/"
// +description:"Sending messages over HTTP"
// +status:"Testing"
type _metaHTTP interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type HTTPOutput struct {
	// Specifies the hostname or IP address and optionally the port number of the web service that can receive log data via HTTP. Use a colon (:) after the address to specify the port number of the server. For example: `http://127.0.0.1:8000`
	URL string `json:"url,omitempty"`
	// Custom HTTP headers to include in the request, for example, `headers("HEADER1: header1", "HEADER2: header2")`. (default: empty)
	Headers []string `json:"headers,omitempty"`
	// The time to wait in seconds before a dead connection is reestablished. (default: 60)
	TimeReopen int `json:"time_reopen,omitempty"`
	// This option sets various options related to TLS encryption, for example, key/certificate files and trusted CA locations. TLS can be used only with tcp-based transport protocols. For details, see [TLS for syslog-ng outputs](../tls/) and the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/).
	TLS *TLS `json:"tls,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Batching parameters
	Batch `json:",inline"`
	// The body of the HTTP request, for example, `body("${ISODATE} ${MESSAGE}")`. You can use strings, macros, and template functions in the body. If not set, it will contain the message received from the source by default.
	Body string `json:"body,omitempty"`
	// The string syslog-ng OSE puts at the beginning of the body of the HTTP request, before the log message.
	BodyPrefix string `json:"body-prefix,omitempty"`
	// The string syslog-ng OSE puts to the end of the body of the HTTP request, after the log message.
	BodySuffix string `json:"body-suffix,omitempty"`
	// By default, syslog-ng OSE separates the log messages of the batch with a newline character.
	Delimiter string `json:"delimiter,omitempty"`
	// Specifies the HTTP method to use when sending the message to the server. `POST | PUT`
	Method string `json:"method,omitempty"`
	// The number of times syslog-ng OSE attempts to send a message to this destination. If syslog-ng OSE could not send a message, it will try again until the number of attempts reaches `retries`, then drops the message.
	Retries int `json:"retries,omitempty"`
	// The username that syslog-ng OSE uses to authenticate on the server where it sends the messages.
	User string `json:"user,omitempty"`
	// The password that syslog-ng OSE uses to authenticate on the server where it sends the messages.
	Password secret.Secret `json:"password,omitempty"`
	// The value of the USER-AGENT header in the messages sent to the server.
	UserAgent string `json:"user-agent,omitempty"`
	// Specifies the number of worker threads (at least 1) that syslog-ng OSE uses to send messages to the server. Increasing the number of worker threads can drastically improve the performance of the destination.
	Workers int `json:"workers,omitempty"`
	// If you receive the following error message during syslog-ng startup, set the `persist-name()` option of the duplicate drivers: `Error checking the uniqueness of the persist names, please override it with persist-name option. Shutting down.` See the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/reference-destination-http-nonjava/#persist-name) for more information.
	PersistName string `json:"persist_name,omitempty"`
	// The number of messages that the output queue can store.
	LogFIFOSize int `json:"log-fifo-size,omitempty"`
	// Sets the maximum number of messages sent to the destination per second. Use this output-rate-limiting functionality only when using disk-buffer as well to avoid the risk of losing messages. Specifying 0 or a lower value sets the output limit to unlimited.
	Timeout int `json:"timeout,omitempty"`
	// Specifies what syslog-ng does with the log message, based on the response code received from the HTTP server. See the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/reference-destination-http-nonjava/#response-action) for more information.
	ResponseAction filter.RawArrowMap `json:"response-action,omitempty"`
}

type Batch struct {
	// Description: Specifies how many lines are flushed to a destination in one batch. The syslog-ng OSE application waits for this number of lines to accumulate and sends them off in a single batch. Increasing this number increases throughput as more messages are sent in a single batch, but also increases message latency.
	// For example, if you set `batch-lines()` to 100, syslog-ng OSE waits for 100 messages.
	BatchLines int `json:"batch-lines,omitempty"`
	// Description: Sets the maximum size of payload in a batch. If the size of the messages reaches this value, syslog-ng OSE sends the batch to the destination even if the number of messages is less than the value of the `batch-lines()` option.
	// Note that if the `batch-timeout()` option is enabled and the queue becomes empty, syslog-ng OSE flushes the messages only if `batch-timeout()` expires, or the batch reaches the limit set in batch-bytes().
	BatchBytes int `json:"batch-bytes,omitempty"`
	// Description: Specifies the time syslog-ng OSE waits for lines to accumulate in the output buffer. The syslog-ng OSE application sends batches to the destinations evenly. The timer starts when the first message arrives to the buffer, so if only few messages arrive, syslog-ng OSE sends messages to the destination at most once every `batch-timeout()` milliseconds.
	BatchTimeout int `json:"batch-timeout,omitempty"`
}

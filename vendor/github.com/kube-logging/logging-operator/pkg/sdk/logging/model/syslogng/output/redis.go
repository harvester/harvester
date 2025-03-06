// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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
)

// +name:"Redis"
// +weight:"200"
type _hugoRedis interface{} //nolint:deadcode,unused

// +docName:"Sending messages from a local network to the Redis server"
/*
Based on the [Redis destination of AxoSyslog core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-redis/).

Available in Logging operator version 4.4 and later.

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: redis
  namespace: default
spec:
  redis:
    host: 127.0.0.1
	port: 6379
	retries: 3
	throttle: 0
	time-reopen: 60
	workers: 1
{{</ highlight >}}

For details on the available options of the output, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-redis/).
*/
type _docRedis interface{} //nolint:deadcode,unused

// +name:"Redis Server Destination"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-redis/"
// +description:"Sending messages from local network to the Redis server"
// +status:"Testing"
type _metaRedis interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type RedisOutput struct {
	// The hostname or IP address of the Redis server. (default: 127.0.0.1)
	Host string `json:"host,omitempty"`
	// The password used for authentication on a password-protected Redis server.
	Auth *secret.Secret `json:"auth,omitempty"`
	// The port number of the Redis server. (default: 6379)
	Port int `json:"port,omitempty"`
	// If syslog-ng OSE cannot send a message, it will try again until the number of attempts reaches `retries()`. (default: 3)
	Retries int `json:"retries,omitempty"`
	//  Sets the maximum number of messages sent to the destination per second. Use this output-rate-limiting functionality only when using disk-buffer as well to avoid the risk of losing messages. Specifying 0 or a lower value sets the output limit to unlimited. (default: 0)
	Throttle int `json:"throttle,omitempty"`
	// The time to wait in seconds before a dead connection is reestablished. (default: 60)
	TimeReopen int `json:"time-reopen,omitempty"`
	// Specifies the number of worker threads (at least 1) that syslog-ng OSE uses to send messages to the server. Increasing the number of worker threads can drastically improve the performance of the destination. (default: 1)
	Workers int `json:"workers,omitempty"`
	// The Redis command to execute, for example, LPUSH, INCR, or HINCRBY. Using the HINCRBY command with an increment value of 1 allows you to create various statistics. For example, the `command("HINCRBY" "${HOST}/programs" "${PROGRAM}" "1")` command counts the number of log messages on each host for each program. (default: "")
	CommandAndArguments []string `json:"command_and_arguments,omitempty" syslog-ng:"ignore"`
	// Internal rendered form of the CommandAndArguments field
	RenderedCommand StringList `json:"command,omitempty"`
	// Batching parameters
	Batch `json:",inline"`
	// The number of messages that the output queue can store.
	LogFIFOSize int `json:"log-fifo-size,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Persistname
	PersistName string `json:"persist_name,omitempty"`
}

// +kubebuilder:object:generate=true
type StringList struct {
	List []string `json:"string-list,omitempty"`
}

func (o *RedisOutput) BeforeRender() {
	if len(o.CommandAndArguments) != 0 {
		o.RenderedCommand = StringList{o.CommandAndArguments[:]}
	}
}

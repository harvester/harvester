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

// +name:"Sumo Logic HTTP"
// +weight:"200"
type _hugoSumologicHTTP interface{} //nolint:deadcode,unused

// +docName:"Storing messages in Sumo Logic over http"
// The `sumologic-http` output sends log records over HTTP to Sumo Logic.
//
// ## Prerequisites
//
// You need a Sumo Logic account to use this output. For details, see the [syslog-ng documentation](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/55#TOPIC-1829118).
//
// ## Example
//
// {{< highlight yaml >}}
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: SyslogNGOutput
// metadata:
//
//	name: test-sumo
//	namespace: default
//
// spec:
//
//	sumologic-http:
//	  batch-lines: 1000
//	  disk_buffer:
//	    disk_buf_size: 512000000
//	    dir: /buffers
//	    reliable: true
//	  body: "$(format-json
//	              --subkeys json.
//	              --exclude json.kubernetes.annotations.*
//	              json.kubernetes.annotations=literal($(format-flat-json --subkeys json.kubernetes.annotations.))
//	              --exclude json.kubernetes.labels.*
//	              json.kubernetes.labels=literal($(format-flat-json --subkeys json.kubernetes.labels.)))"
//	  collector:
//	    valueFrom:
//	      secretKeyRef:
//	        key: token
//	        name: sumo-collector
//	  deployment: us2
//	  headers:
//	  - 'X-Sumo-Name: source-name'
//	  - 'X-Sumo-Category: source-category'
//	  tls:
//	    use-system-cert-store: true
//
// {{</ highlight >}}
type _docSumologicHTTP interface{} //nolint:deadcode,unused

// +name:"Sumo Logic HTTP"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/55"
// +description:"Storing messages in Sumo Logic over http"
// +status:"Testing"
type _metaSumologicHTTP interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type SumologicHTTPOutput struct {
	// The Cloud Syslog Cloud Token that you received from the Sumo Logic service while configuring your cloud syslog source. (default: empty)
	Collector *secret.Secret `json:"collector,omitempty"`
	// This option specifies your Sumo Logic deployment.https://help.sumologic.com/APIs/General-API-Information/Sumo-Logic-Endpoints-by-Deployment-and-Firewall-Security  (default: empty)
	Deployment string `json:"deployment,omitempty"`
	// Custom HTTP headers to include in the request, for example, headers("HEADER1: header1", "HEADER2: header2").  (default: empty)
	Headers []string `json:"headers,omitempty"`
	// The time to wait in seconds before a dead connection is reestablished. (default: 60)
	TimeReopen int `json:"time_reopen,omitempty"`
	// This option sets various options related to TLS encryption, for example, key/certificate files and trusted CA locations. TLS can be used only with tcp-based transport protocols. For details, see [TLS for syslog-ng outputs](../tls/) and the [syslog-ng documentation](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#TOPIC-1829193).
	TLS *TLS `json:"tls,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer   *DiskBuffer    `json:"disk_buffer,omitempty"`
	Body         string         `json:"body,omitempty"`
	URL          *secret.Secret `json:"url,omitempty"`
	BatchLines   int            `json:"batch-lines,omitempty"`
	BatchBytes   int            `json:"batch-bytes,omitempty"`
	BatchTimeout int            `json:"batch-timeout,omitempty"`
	PersistName  string         `json:"persist_name,omitempty"`
}

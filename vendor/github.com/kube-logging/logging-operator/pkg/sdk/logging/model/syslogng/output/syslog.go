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

// +name:"Syslog (RFC5424) output"
// +weight:"200"
type _hugoSyslogOutput interface{} //nolint:deadcode,unused

// +docName:"Syslog output configuration"
// The `syslog` output sends log records over a socket using the Syslog protocol (RFC 5424).
//
// {{< highlight yaml >}}
//
//	spec:
//	  syslog:
//	    host: 10.12.34.56
//	    transport: tls
//	    tls:
//	      ca_file:
//	        mountFrom:
//	          secretKeyRef:
//	            name: tls-secret
//	            key: ca.crt
//	      cert_file:
//	        mountFrom:
//	          secretKeyRef:
//	            name: tls-secret
//	            key: tls.crt
//	      key_file:
//	        mountFrom:
//	          secretKeyRef:
//	            name: tls-secret
//	            key: tls.key
//
// {{</ highlight >}}
//
// The following example also configures disk-based buffering for the output. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/).
//
// {{< highlight yaml >}}
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: SyslogNGOutput
// metadata:
//
//	name: test
//	namespace: default
//
// spec:
//
//	syslog:
//	  host: 10.20.9.89
//	  port: 601
//	  disk_buffer:
//	    disk_buf_size: 512000000
//	    dir: /buffer
//	    reliable: true
//	  template: "$(format-json
//	              --subkeys json.
//	              --exclude json.kubernetes.labels.*
//	              json.kubernetes.labels=literal($(format-flat-json --subkeys json.kubernetes.labels.)))\n"
//	  tls:
//	    ca_file:
//	      mountFrom:
//	        secretKeyRef:
//	          key: ca.crt
//	          name: syslog-tls-cert
//	    cert_file:
//	      mountFrom:
//	        secretKeyRef:
//	          key: tls.crt
//	          name: syslog-tls-cert
//	    key_file:
//	      mountFrom:
//	        secretKeyRef:
//	          key: tls.key
//	          name: syslog-tls-cert
//	  transport: tls
//
// {{</ highlight >}}
//
// For details on the available options of the output, see the [syslog-ng documentation](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#TOPIC-1829124).
type _docSyslogOutput interface{} //nolint:deadcode,unused

// +name:"Syslog output configuration"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/32#kanchor2338"
// +description:"Syslog output configuration"
// +status:"Testing"
type _metaSyslogOutput interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// Documentation: https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#TOPIC-1829124
type SyslogOutput struct {
	// Address of the destination host
	Host string `json:"host,omitempty" syslog-ng:"pos=0"`
	// The port number to connect to. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor895)
	Port int `json:"port,omitempty"`
	// Specifies the protocol used to send messages to the destination server. [more information]() [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor911)
	Transport string `json:"transport,omitempty"`
	// By default, syslog-ng OSE closes destination sockets if it receives any input from the socket (for example, a reply). If this option is set to no, syslog-ng OSE just ignores the input, but does not close the socket. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor859)
	CloseOnInput *bool `json:"close_on_input,omitempty"`
	// Flags influence the behavior of the destination driver. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor877)
	Flags []string `json:"flags,omitempty"`
	// Specifies how many lines are flushed to a destination at a time. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor880)
	FlushLines int `json:"flush_lines,omitempty"`
	// Enables keep-alive messages, keeping the socket open. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor897)
	SoKeepalive *bool `json:"so_keepalive,omitempty"`
	// Specifies the number of seconds syslog-ng waits for identical messages. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor901)
	Suppress int `json:"suppress,omitempty"`
	// Specifies a template defining the logformat to be used in the destination. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor905) (default: 0)
	Template string `json:"template,omitempty"`
	// Turns on escaping for the ', ", and backspace characters in templated output files. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor906)
	TemplateEscape *bool `json:"template_escape,omitempty"`
	// Sets various options related to TLS encryption, for example, key/certificate files and trusted CA locations. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor910)
	TLS *TLS `json:"tls,omitempty"`
	// Override the global timestamp format (set in the global ts-format() parameter) for the specific destination. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/56#kanchor912)
	TSFormat string `json:"ts_format,omitempty"`
	// Enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/).
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Unique name for the syslog-ng driver [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.16/administration-guide/persist-name)
	PersistName string `json:"persist_name,omitempty"`
}

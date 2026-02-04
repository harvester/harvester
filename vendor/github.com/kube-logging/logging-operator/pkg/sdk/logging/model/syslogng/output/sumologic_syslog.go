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

// +name:"Sumo Logic Syslog"
// +weight:"200"
type _hugoSumologicSyslog interface{} //nolint:deadcode,unused

// +docName:"Storing messages in Sumo Logic over syslog"
/*
The `sumologic-syslog` output sends log records over HTTP to Sumo Logic. For details on the available options of the output, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-sumologic-intro/destination-sumologic-options/).

## Prerequisites

You need a Sumo Logic account to use this output. For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-sumologic-intro/).
*/
type _docSumologicSyslog interface{} //nolint:deadcode,unused

// +name:"Sumo Logic Syslog"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-sumologic-intro/destination-sumologic-syslog/"
// +description:"Storing messages in Sumo Logic over syslog"
// +status:"Testing"
type _metaSumologicSyslog interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type SumologicSyslogOutput struct {
	// This option sets the port number of the Sumo Logic server to connect to. (default: 6514)
	Port int `json:"port,omitempty"`
	// This option specifies your Sumo Logic deployment.https://help.sumologic.com/APIs/General-API-Information/Sumo-Logic-Endpoints-by-Deployment-and-Firewall-Security  (default: empty)
	Deployment string `json:"deployment,omitempty"`
	//  This option specifies the list of tags to add as the tags fields of Sumo Logic messages. If not specified, syslog-ng OSE automatically adds the tags already assigned to the message. If you set the tag() option, only the tags you specify will be added to the messages. (default: tag)
	Tag string `json:"tag,omitempty"`
	// The Cloud Syslog Cloud Token that you received from the Sumo Logic service while configuring your cloud syslog source. https://help.sumologic.com/03Send-Data/Sources/02Sources-for-Hosted-Collectors/Cloud-Syslog-Source#configure-a-cloud%C2%A0syslog%C2%A0source
	Token int `json:"token,omitempty"`
	// This option sets various options related to TLS encryption, for example, key/certificate files and trusted CA locations. TLS can be used only with tcp-based transport protocols. For details, see [TLS for syslog-ng outputs](../tls/) and the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/).
	TLS *TLS `json:"tls,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer  *DiskBuffer `json:"disk_buffer,omitempty"`
	PersistName string      `json:"persist_name,omitempty"`
}

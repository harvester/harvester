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

// +name:"MQTT"
// +weight:"200"
type _hugoMQTT interface{} //nolint:deadcode,unused

// +docName:"Sending messages from a local network to an MQTT broker"
//
// ## Prerequisites
//
// ## Example
//
// {{< highlight yaml >}}
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: SyslogNGOutput
// metadata:
//
//	name: mqtt
//	namespace: default
//
// spec:
//
//	mqtt:
//	  address: tcp://mosquitto:1883
//	  template: |
//	    $(format-json --subkeys json~ --key-delimiter ~)
//	  topic: test/demo
//
// {{</ highlight >}}
type _docMQTT interface{} //nolint:deadcode,unused

// +name:"MQTT Destination"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/45#TOPIC-1829079"
// +description:"Sending messages over MQTT Protocol"
// +status:"Testing"
type _metaMQTT interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type MQTT struct {
	// Address of the destination host
	Address string `json:"address,omitempty"`
	// Topic defines in which topic syslog-ng stores the log message. You can also use templates here, and use, for example, the $HOST macro in the topic name hierarchy.
	Topic string `json:"topic,omitempty"`
	// fallback-topic is used when syslog-ng cannot post a message to the originally defined topic (which can include invalid characters coming from templates).
	FallbackTopic string `json:"fallback-topic,omitempty"`
	// Template where you can configure the message template sent to the MQTT broker. By default, the template is: “$ISODATE $HOST $MSGHDR$MSG”
	Template string `json:"template,omitempty"`
	// qos stands for quality of service and can take three values in the MQTT world. Its default value is 0, where there is no guarantee that the message is ever delivered.
	QOS int `json:"qos,omitempty"`
}

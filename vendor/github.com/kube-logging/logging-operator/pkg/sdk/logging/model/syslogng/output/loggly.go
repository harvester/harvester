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

// +name:"Loggly output"
// +weight:"200"
type _hugoLoggly interface{} //nolint:deadcode,unused

// +docName:"Loggly output plugin for syslog-ng"
// The `loggly()` destination sends log messages to the [Loggly](https://www.loggly.com/) Logging-as-a-Service provider. You can send log messages over TCP, or encrypted with TLS. For details, see the [syslog-ng documentation](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/43#TOPIC-1829072).
//
// ## Prerequisites
//
// You need a Loggly account and your user token to use this output.
type _docLoggly interface{} //nolint:deadcode,unused

// +name:"Loggly"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/43#TOPIC-1829072"
// +description:"Send your logs to loggly"
// +status:"Testing"
type _metaLoggly interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// Documentation: https://github.com/syslog-ng/syslog-ng/blob/master/scl/loggly/loggly.conf

type Loggly struct {
	// Address of the destination host
	Host string `json:"host,omitempty"`
	// Event tag [more information](https://documentation.solarwinds.com/en/success_center/loggly/content/admin/tags.htm)
	Tag string `json:"tag,omitempty"`
	// Your Customer Token that you received from Loggly [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/43#loggly-option-token)
	Token *secret.Secret `json:"token"`
	// syslog output configuration
	SyslogOutput `json:",inline"`
}

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

// +name:"MongoDB"
// +weight:"200"
type _hugoMongoDB interface{} //nolint:deadcode,unused

// +docName:"Sending messages from a local network to an MongoDB database"
/*
Based on the [MongoDB destination of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/).

Available in Logging operator version 4.4 and later.

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: mongodb
  namespace: default
spec:
  mongodb:
    collection: syslog
    uri: "mongodb://mongodb-endpoint/syslog?wtimeoutMS=60000&socketTimeoutMS=60000&connectTimeoutMS=60000"
    value_pairs: scope("selected-macros" "nv-pairs")
{{</ highlight >}}

For more information, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/).
*/
type _docMongoDB interface{} //nolint:deadcode,unused

// +name:"MongoDB Destination"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/"
// +description:"Sending messages into MongoDB Server"
// +status:"Testing"
type _metaMongoDB interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type MongoDB struct {
	// The name of the MongoDB collection where the log messages are stored (collections are similar to SQL tables). Note that the name of the collection must not start with a dollar sign ($), and that it may contain dot (.) characters.
	Collection string `json:"collection"`
	// Defines the folder where the disk-buffer files are stored.
	Dir string `json:"dir,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
	// Connection string used for authentication.
	// See the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/reference-destination-mongodb/#mongodb-option-uri)
	Uri *secret.Secret `json:"uri,omitempty"`
	// Creates structured name-value pairs from the data and metadata of the log message. (default: `"scope("selected-macros" "nv-pairs")"`)
	ValuePairs ValuePairs `json:"value_pairs,omitempty"`
	// Batching parameters
	Batch `json:",inline"`
	// Bulk operation related options
	Bulk `json:",inline"`
	// The number of messages that the output queue can store.
	LogFIFOSize int `json:"log-fifo-size,omitempty"`
	// If you receive the following error message during syslog-ng startup, set the `persist-name()` option of the duplicate drivers:
	// `Error checking the uniqueness of the persist names, please override it with persist-name option. Shutting down.`
	// See the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-http-nonjava/reference-destination-http-nonjava/#persist-name) for more information.
	PersistName string `json:"persist_name,omitempty"`
	// The number of times syslog-ng OSE attempts to send a message to this destination. If syslog-ng OSE could not send a message, it will try again until the number of attempts reaches retries, then drops the message.
	Retries int `json:"retries,omitempty"`
	// The time to wait in seconds before a dead connection is reestablished. (default: 60)
	TimeReopen int `json:"time_reopen,omitempty"`
	// 	Description: Sets the write concern mode of the MongoDB operations, for both bulk and single mode.
	// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/reference-destination-mongodb/#mongodb-option-write-concern).
	// +kubebuilder:validation:Enum=unacked;acked;majority
	WriteConcern RawString `json:"write_concern,omitempty"`
}

// +kubebuilder:object:generate=true
// Bulk operation related options.
// For details, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-mongodb/reference-destination-mongodb/#mongodb-option-bulk).
type Bulk struct {
	// Enables bulk insert mode. If disabled, each messages is inserted individually. (default: yes)
	Bulk *bool `json:"bulk,omitempty"`
	// If set to yes, it disables MongoDB bulk operations validation mode. (default: no)
	BulkByPassValidation *bool `json:"bulk_bypass_validation,omitempty"`
	// Description: Enables unordered bulk operations mode. (default: no)
	BulkUnordered *bool `json:"bulk_unordered,omitempty"`
}

// +kubebuilder:object:generate=true
type WriteConcern int

const (
	Unacked  = "unacked"
	Acked    = "acked"
	Majority = "majority"
)

// +kubebuilder:object:generate=true
// TODO move this to a common module once it is used in more places
type ValuePairs struct {
	Scope   RawString `json:"scope,omitempty"`
	Exclude RawString `json:"exclude,omitempty"`
	Key     RawString `json:"key,omitempty"`
	Pair    RawString `json:"pair,omitempty"`
}

type RawString string

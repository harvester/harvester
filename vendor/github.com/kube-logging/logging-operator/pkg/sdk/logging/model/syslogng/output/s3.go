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

// +name:"S3"
// +weight:"200"
type _hugoS3 interface{} //nolint:deadcode,unused

// +docName:"Sending messages from a local network to a S3 (compatible) server"
/*
Sends messages from a local network to a S3 (compatible) server. For more information, see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-s3/).

Available in Logging operator version 4.4 and later.

## Example
{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: s3
spec:
  s3:
    url: "https://s3-endpoint"
    bucket: "s3bucket-name"
    access_key:
      valueFrom:
        secretKeyRef:
          name: s3
          key: access-key
    secret_key:
      valueFrom:
        secretKeyRef:
          name: s3
          key: secret-key
    object_key: "path/to/my-logs/${HOST}"
{{</ highlight >}}

For available macros like `$PROGRAM` and `$HOST`,  see the [documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-manipulating-messages/customizing-message-format/reference-macros/).
*/
type _docS3 interface{} //nolint:deadcode,unused

// +name:"S3 Destination"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/destination-s3/"
// +description:"Sending messages from a local network to a S3 (compatible) server"
// +status:"Testing"
type _metaS3 interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type S3Output struct {
	// The hostname or IP address of the S3 server.
	Url string `json:"url,omitempty"`
	// The bucket name of the S3 server.
	Bucket string `json:"bucket,omitempty"`
	// The access_key for the S3 server.
	AccessKey *secret.Secret `json:"access_key,omitempty"`
	// The secret_key for the S3 server.
	SecretKey *secret.Secret `json:"secret_key,omitempty"`
	// The object_key for the S3 server.
	ObjectKey string `json:"object_key,omitempty"`
	// Set object_key_timestamp
	ObjectKeyTimestamp RawString `json:"object_key_timestamp,omitempty"`
	// Template
	Template RawString `json:"template,omitempty"`
	// Enable or disable compression. (default: false)
	Compression *bool `json:"compression,omitempty"`
	// Set the compression level (1-9). (default: 9)
	CompressLevel int `json:"compresslevel,omitempty"`
	// Set the chunk size. (default: 5MiB)
	ChunkSize int `json:"chunk_size,omitempty"`
	// Set the maximum object size size. (default: 5120GiB)
	MaxObjectSize int `json:"max_object_size,omitempty"`
	// Set the number of upload threads. (default: 8)
	UploadThreads int `json:"upload_threads,omitempty"`
	// Set the maximum number of pending uploads. (default: 32)
	MaxPendingUploads int `json:"max_pending_uploads,omitempty"`
	// Set the number of seconds for flush period. (default: 60)
	FlushGracePeriod int `json:"flush_grace_period,omitempty"`
	// Set the region option.
	Region string `json:"region,omitempty"`
	// Set the storage_class option.
	StorageClass string `json:"storage_class,omitempty"`
	// Set the canned_acl option.
	CannedAcl string `json:"canned_acl,omitempty"`
	// The number of messages that the output queue can store.
	LogFIFOSize int `json:"log-fifo-size,omitempty"`
	// Persistname
	PersistName string `json:"persist_name,omitempty"`
	// The number of times syslog-ng OSE attempts to send a message to this destination. If syslog-ng OSE could not send a message, it will try again until the number of attempts reaches retries, then drops the message.
	Retries int `json:"retries,omitempty"`
	//  Sets the maximum number of messages sent to the destination per second. Use this output-rate-limiting functionality only when using disk-buffer as well to avoid the risk of losing messages. Specifying 0 or a lower value sets the output limit to unlimited. (default: 0)
	Throttle int `json:"throttle,omitempty"`
}

// Copyright © 2024 Kube logging authors
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

// +name:"Elasticsearch datastream"
// +weight:"200"
type _hugoElasticsearchDatastream interface{} //nolint:deadcode,unused

// +docName:"Sending messages over Elasticsearch datastreams"
/*
Based on the [ElasticSearch datastream destination of AxoSyslog core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-elasticsearch-datastream/).

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: elasticsearc-hdatastream
spec:
  elasticsearch-datastream:
    url: "https://elastic-endpoint:9200/my-data-stream/_bulk"
    user: "username"
    password:
      valueFrom:
        secretKeyRef:
          name: elastic
          key: password
{{</ highlight >}}
*/
type _docSElasticsearchDatastream interface{} //nolint:deadcode,unused

// +name:"Elasticsearch datastream"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-elasticsearch-datastream/"
// +description:"Sending messages over Elasticsearch datastreams"
// +status:"Testing"
type _metaElasticsearchDatastream interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type ElasticsearchDatastreamOutput struct {
	HTTPOutput `json:",inline"`
	// Arguments to the `$format-json()` template function.
	// Default: `"--scope rfc5424 --exclude DATE --key ISODATE @timestamp=${ISODATE}"`
	Record string `json:"record,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer *DiskBuffer `json:"disk_buffer,omitempty"`
}

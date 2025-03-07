// Copyright © 2023 Kube logging authors
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

import "fmt"

// +name:"Elasticsearch"
// +weight:"200"
type _hugoElasticsearch interface{} //nolint:deadcode,unused

// +docName:"Sending messages over Elasticsearch"
/*
Based on the [ElasticSearch destination of AxoSyslog core](https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-elasticsearch-http/).

## Example

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: SyslogNGOutput
metadata:
  name: elasticsearch
spec:
  elasticsearch:
    url: "https://elastic-search-endpoint:9200/_bulk"
    index: "indexname"
    type: ""
    user: "username"
    password:
      valueFrom:
        secretKeyRef:
          name: elastic
          key: password
{{</ highlight >}}
*/
type _docSElasticsearch interface{} //nolint:deadcode,unused

// +name:"Elasticsearch"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-destinations/configuring-destinations-elasticsearch-http/"
// +description:"Sending messages over Elasticsearch"
// +status:"Testing"
type _metaElasticsearch interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type ElasticsearchOutput struct {
	HTTPOutput `json:",inline"`
	// Name of the data stream, index, or index alias to perform the action on.
	Index string `json:"index,omitempty"`
	// The document type associated with the operation. Elasticsearch indices now support a single document type: `_doc`
	Type *string `json:"type,omitempty"`
	// The template to format the record itself inside the payload body
	Template string `json:"template,omitempty"`
	// The document ID. If no ID is specified, a document ID is automatically generated.
	CustomID string `json:"custom_id,omitempty"`
	// Set the prefix for logs in logstash format. If set, then the Index field will be ignored.
	LogstashPrefix string `json:"logstash_prefix,omitempty" syslog-ng:"ignore"`
	// Set the separator between LogstashPrefix and LogStashDateformat. Default: "-"
	LogstashPrefixSeparator string `json:"logstash_prefix_separator,omitempty" syslog-ng:"ignore"`
	// Set the suffix for logs in logstash format. Default: `"${YEAR}.${MONTH}.${DAY}"`
	LogStashSuffix string `json:"logstash_suffix,omitempty" syslog-ng:"ignore"`
}

func (o *ElasticsearchOutput) BeforeRender() {
	if o.LogstashPrefixSeparator == "" {
		o.LogstashPrefixSeparator = "-"
	}

	if o.LogStashSuffix == "" {
		o.LogStashSuffix = "${YEAR}.${MONTH}.${DAY}"
	}

	if o.LogstashPrefix != "" {
		o.Index = fmt.Sprintf("%s%s%s", o.LogstashPrefix, o.LogstashPrefixSeparator, o.LogStashSuffix)
	}
}

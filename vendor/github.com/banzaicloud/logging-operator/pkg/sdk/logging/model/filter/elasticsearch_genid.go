// Copyright © 2019 Banzai Cloud
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

package filter

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +kubebuilder:object:generate=true
type ElasticsearchGenId struct {
	// You can specify generated hash storing key.
	HashIdKey string `json:"hash_id_key,omitempty"`
	// You can specify to use tag for hash generation seed.
	IncludeTagInSeed bool `json:"include_tag_in_seed,omitempty"`
	// You can specify to use time for hash generation seed.
	IncludeTimeInSeed bool `json:"include_time_in_seed,omitempty"`
	// You can specify to use record in events for hash generation seed. This parameter should be used with record_keys parameter in practice.
	UseRecordAsSeed bool `json:"use_record_as_seed,omitempty"`
	// You can specify keys which are record in events for hash generation seed. This parameter should be used with use_record_as_seed parameter in practice.
	RecordKeys string `json:"record_keys,omitempty"`
	// You can specify to use entire record in events for hash generation seed.
	UseEntireRecord bool `json:"use_entire_record,omitempty"`
	// You can specify separator charactor to creating seed for hash generation.
	Separator string `json:"separator,omitempty"`
	// You can specify hash algorithm. Support algorithms md5, sha1, sha256, sha512. Default: sha1
	HashType string `json:"hash_type,omitempty"`
}

// ## Example `Elasticsearch Genid` filter configurations
// ```yaml
//apiVersion: logging.banzaicloud.io/v1beta1
//kind: Flow
//metadata:
//  name: demo-flow
//spec:
//  filters:
//    - elasticsearch_genid:
//        hash_id_key: gen_id
//  selectors: {}
//  localOutputRefs:
//    - demo-output
// ```
//
// #### Fluentd Config Result
// ```yaml
//<filter **>
//  @type elasticsearch_genid
//  @id test_elasticsearch_genid
//  hash_id_key gen_id
//</filter>
// ```

func NewElasticsearchGenId() *ElasticsearchGenId {
	return &ElasticsearchGenId{}
}

func (c *ElasticsearchGenId) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "elasticsearch_genid"
	return types.NewFlatDirective(types.PluginMeta{
		Type:      pluginType,
		Directive: "filter",
		Tag:       "**",
		Id:        id,
	}, c, secretLoader)
}

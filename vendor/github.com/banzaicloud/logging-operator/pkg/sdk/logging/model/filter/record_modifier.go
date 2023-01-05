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

// +name:"Record Modifier"
// +weight:"200"
type _hugoRecordModifier interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Record Modifier](https://github.com/repeatedly/fluent-plugin-record-modifier)"
// Modify each event record.
type _docRecordModifier interface{} //nolint:deadcode,unused

// +name:"Record Modifier"
// +url:"https://github.com/repeatedly/fluent-plugin-record-modifier"
// +version:"2.1.0"
// +description:"Modify each event record."
// +status:"GA"
type _metaRecordModifier interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type RecordModifier struct {
	// Prepare values for filtering in configure phase. Prepared values can be used in <record>. You can write any ruby code.
	PrepareValues string `json:"prepare_value,omitempty"`
	// Fluentd including some plugins treats logs as a BINARY by default to forward. To overide that, use a target encoding or a from:to encoding here.
	CharEncoding string `json:"char_encoding,omitempty"`
	// A comma-delimited list of keys to delete
	RemoveKeys string `json:"remove_keys,omitempty"`
	// This is exclusive with remove_keys
	WhitelistKeys string `json:"whitelist_keys,omitempty"`
	// Replace specific value for keys
	Replaces []Replace `json:"replaces,omitempty"`
	// Add records docs at: https://github.com/repeatedly/fluent-plugin-record-modifier
	// Records are represented as maps: `key: value`
	Records []Record `json:"records,omitempty"`
}

// ## Example `Record Modifier` filter configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Flow
// metadata:
//
//	name: demo-flow
//
// spec:
//
//	filters:
//	  - record_modifier:
//	      records:
//	      - foo: "bar"
//	selectors: {}
//	localOutputRefs:
//	  - demo-output
//
// ```
//
// #### Fluentd Config Result
// ```yaml
// <filter **>
//
//	@type record_modifier
//	@id test_record_modifier
//	<record>
//	  foo bar
//	</record>
//
// </filter>
// ```
type _expRecordModifier interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[Replace Directive](https://github.com/repeatedly/fluent-plugin-record-modifier#replace_keys_value)"
// Specify replace rule. This directive contains three parameters.
type Replace struct {
	// Key to search for
	Key string `json:"key"`
	// Regular expression
	Expression string `json:"expression"`
	// Value to replace with
	Replace string `json:"replace"`
}

func (r *Replace) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	meta := types.PluginMeta{
		Directive: "replace",
	}
	return types.NewFlatDirective(meta, r, secretLoader)
}

func (r *RecordModifier) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "record_modifier"
	recordModifier := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(r); err != nil {
		return nil, err
	} else {
		recordModifier.Params = params
	}
	for _, replace := range r.Replaces {
		if meta, err := replace.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			recordModifier.SubDirectives = append(recordModifier.SubDirectives, meta)
		}
	}
	for _, record := range r.Records {
		if meta, err := record.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			recordModifier.SubDirectives = append(recordModifier.SubDirectives, meta)
		}
	}
	return recordModifier, nil
}

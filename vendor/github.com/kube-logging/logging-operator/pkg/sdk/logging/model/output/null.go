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

package output

import (
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Null"
// +weight:"200"
type _hugoNull interface{} //nolint:deadcode,unused

// +name:"Null"
// +url:"https://docs.fluentd.org/output/null"
// +version:"more info"
// +description:"Null output plugin just throws away events."
// +status:"GA"
type _metaNull interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

type NullOutputConfig struct {
}

func NewNullOutputConfig() *NullOutputConfig {
	return &NullOutputConfig{}
}

func (c *NullOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "null"
	return types.NewFlatDirective(types.PluginMeta{
		Type:      pluginType,
		Directive: "match",
		Tag:       "**",
		Id:        id,
	}, c, secretLoader)
}

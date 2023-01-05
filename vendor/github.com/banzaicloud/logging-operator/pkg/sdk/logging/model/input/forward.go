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

package input

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/common"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +kubebuilder:object:generate=true

type ForwardInputConfig struct {
	Port                string `json:"port,omitempty" plugin:"default:24240"`
	Bind                string `json:"bind,omitempty" plugin:"default:0.0.0.0"`
	Tag                 string `json:"tag,omitempty"`
	AddTagPrefix        string `json:"add_tag_prefix,omitempty"`
	LingerTimeout       int    `json:"linger_timeout,omitempty"`
	ResolveHostname     *bool  `json:"resolve_hostname,omitempty"`
	DenyKeepalive       *bool  `json:"deny_keepalive,omitempty"`
	SendKeepalivePacket *bool  `json:"send_keepalive_packet,omitempty"`
	ChunkSizeLimit      string `json:"chunk_size_limit,omitempty"`
	ChunkSizeWarnLimit  string `json:"chunk_size_warn_limit,omitempty"`
	SkipInvalidEvent    *bool  `json:"skip_invalid_event,omitempty"`
	SourceAddressKey    string `json:"source_address_key,omitempty"`
	SourceHostnameKey   string `json:"sourceHostnameKey,omitempty"`

	Transport *common.Transport `json:"transport,omitempty"`
	Security  *common.Security  `json:"security,omitempty"`
}

func NewForwardInputConfig() *ForwardInputConfig {
	return &ForwardInputConfig{}
}

func (f *ForwardInputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "forward"
	pluginID := id + "_" + pluginType
	forward := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "source",
			Id:        pluginID,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(f); err != nil {
		return nil, err
	} else {
		forward.Params = params
	}
	if f.Transport != nil {
		if transport, err := f.Transport.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			forward.SubDirectives = append(forward.SubDirectives, transport)
		}
	}
	if f.Security != nil {
		if security, err := f.Security.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			forward.SubDirectives = append(forward.SubDirectives, security)
		}
	}
	return forward, nil
}

// Copyright © 2021 Cisco Systems, Inc. and/or its affiliates
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

package common

import (
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Security"
// +weight:"200"
type _hugoSecurity interface{} //nolint:deadcode,unused

// +name:"Security"
type _metaSecurity interface{} //nolint:deadcode,unused

type Security struct {
	// Hostname
	SelfHostname string `json:"self_hostname"`
	// Shared key for authentication.
	SharedKey string `json:"shared_key"`
	// If true, use user based authentication.
	UserAuth bool `json:"user_auth,omitempty"`
	// Allow anonymous source. <client> sections are required if disabled.
	AllowAnonymousSource bool `json:"allow_anonymous_source,omitempty"`
}

func (s *Security) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "security",
	}, s, secretLoader)
}

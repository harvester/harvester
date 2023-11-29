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

// +name:"Transport"
// +weight:"200"
type _hugoTransport interface{} //nolint:deadcode,unused

// +name:"Transport"
type _metaTransport interface{} //nolint:deadcode,unused

type Transport struct {
	// Protocol Default: :tcp
	Protocol string `json:"protocol,omitempty"`
	// Version Default: 'TLSv1_2'
	Version string `json:"version,omitempty"`
	// Ciphers Default: "ALL:!aNULL:!eNULL:!SSLv2"
	Ciphers string `json:"ciphers,omitempty"`
	// Use secure connection when use tls) Default: false
	Insecure bool `json:"insecure,omitempty"`
	// Specify path to CA certificate file
	CaPath string `json:"ca_path,omitempty"`
	// Specify path to Certificate file
	CertPath string `json:"cert_path,omitempty"`
	// Specify path to private Key file
	PrivateKeyPath string `json:"private_key_path,omitempty"`
	// public CA private key passphrase contained path
	PrivateKeyPassphrase string `json:"private_key_passphrase,omitempty"`
	// When this is set Fluentd will check all incoming HTTPS requests
	// for a client certificate signed by the trusted CA, requests that
	// don't supply a valid client certificate will fail.
	ClientCertAuth bool `json:"client_cert_auth,omitempty"`
	// Specify private CA contained path
	CaCertPath string `json:"ca_cert_path,omitempty"`
	// private CA private key contained path
	CaPrivateKeyPath string `json:"ca_private_key_path,omitempty"`
	// private CA private key passphrase contained path
	CaPrivateKeyPassphrase string `json:"ca_private_key_passphrase,omitempty"`
}

func (t *Transport) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "transport",
		Tag:       "tls",
	}, t, secretLoader)
}

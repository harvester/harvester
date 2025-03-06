// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

import "github.com/cisco-open/operator-tools/pkg/secret"

// +name:"TLS config for syslog-ng outputs"
// +weight:"200"
type _hugoTLS interface{} //nolint:deadcode,unused

// +docName:"TLS config for syslog-ng outputs"
// For details on how TLS configuration works in syslog-ng, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/).
type _docTLS interface{} //nolint:deadcode,unused

// +name:"TLS config for syslog-ng outputs"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/"
// +description:"TLS config for syslog-ng outputs"
// +status:"Testing"
type _metaTLS interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type TLS struct {
	// The name of a directory that contains a set of trusted CA certificates in PEM format. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#ca-dir).
	CaDir *secret.Secret `json:"ca_dir,omitempty"`
	// The name of a file that contains a set of trusted CA certificates in PEM format. (Optional) For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#ca-file).
	CaFile *secret.Secret `json:"ca_file,omitempty"`
	// The name of a file that contains an unencrypted private key in PEM format, suitable as a TLS key. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#key-file).
	KeyFile *secret.Secret `json:"key_file,omitempty"`
	// Name of a file, that contains an X.509 certificate (or a certificate chain) in PEM format, suitable as a TLS certificate, matching the private key set in the key-file() option. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#cert-file).
	CertFile *secret.Secret `json:"cert_file,omitempty"`
	// Verification method of the peer. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#tls-options-peer-verify).
	PeerVerify *bool `json:"peer_verify,omitempty"`
	// Use the certificate store of the system for verifying HTTPS certificates. For details, see the [AxoSyslog Core documentation](https://curl.se/docs/sslcerts.html).
	UseSystemCertStore *bool `json:"use-system-cert-store,omitempty"`
	// Description: Specifies the cipher, hash, and key-exchange algorithms used for the encryption, for example, ECDHE-ECDSA-AES256-SHA384. The list of available algorithms depends on the version of OpenSSL used to compile syslog-ng.
	CipherSuite string `json:"cipher-suite,omitempty"`
	// Configure required TLS version. Accepted values: [sslv3, tlsv1, tlsv1_0, tlsv1_1, tlsv1_2, tlsv1_3]
	// +kubebuilder:validation:Enum=sslv3;tlsv1;tlsv1_0;tlsv1_1;tlsv1_2;tlsv1_3
	SslVersion string `json:"ssl_version,omitempty"`
}

// +kubebuilder:object:generate=true
type GrpcTLS struct {
	// The name of a file that contains a set of trusted CA certificates in PEM format. (Optional) For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#ca-file).
	CaFile *secret.Secret `json:"ca_file,omitempty"`
	// The name of a file that contains an unencrypted private key in PEM format, suitable as a TLS key. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#key-file).
	KeyFile *secret.Secret `json:"key_file,omitempty"`
	// Name of a file, that contains an X.509 certificate (or a certificate chain) in PEM format, suitable as a TLS certificate, matching the private key set in the key-file() option. For details, see the [AxoSyslog Core documentation](https://axoflow.com/docs/axosyslog-core/chapter-encrypted-transport-tls/tlsoptions/#cert-file).
	CertFile *secret.Secret `json:"cert_file,omitempty"`
}

// Copyright © 2022 Banzai Cloud
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

import "github.com/banzaicloud/operator-tools/pkg/secret"

// +name:"TLS config for syslog-ng outputs"
// +weight:"200"
type _hugoTLS interface{} //nolint:deadcode,unused

// +docName:"TLS config for syslog-ng outputs"
// More info at https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/32#kanchor2338
type _docTLS interface{} //nolint:deadcode,unused

// +name:"TLS config for syslog-ng outputs"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/32#kanchor2338"
// +description:"TLS config for syslog-ng outputs"
// +status:"Testing"
type _metaTLS interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type TLS struct {
	// The name of a directory that contains a set of trusted CA certificates in PEM format. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#kanchor3142)
	CaDir *secret.Secret `json:"ca_dir,omitempty"`
	// The name of a file that contains a set of trusted CA certificates in PEM format. (Optional) [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#kanchor3144)
	CaFile *secret.Secret `json:"ca_file,omitempty"`
	// The name of a file that contains an unencrypted private key in PEM format, suitable as a TLS key. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#kanchor3163)
	KeyFile *secret.Secret `json:"key_file,omitempty"`
	// Name of a file, that contains an X.509 certificate (or a certificate chain) in PEM format, suitable as a TLS certificate, matching the private key set in the key-file() option. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#kanchor3146)
	CertFile *secret.Secret `json:"cert_file,omitempty"`
	// Verification method of the peer. [more information](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/73#tls-options-peer-verify)
	PeerVerify string `json:"peer_verify,omitempty"`
	// Use the certificate store of the system for verifying HTTPS certificates. [more information](https://curl.se/docs/sslcerts.html)
	UseSystemCertStore *bool `json:"use-system-cert-store,omitempty"`
	// Description: Specifies the cipher, hash, and key-exchange algorithms used for the encryption, for example, ECDHE-ECDSA-AES256-SHA384. The list of available algorithms depends on the version of OpenSSL used to compile syslog-ng OSE
	CipherSuite string `json:"cipher-suite,omitempty"`
}

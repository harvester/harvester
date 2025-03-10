// Copyright © 2020 Banzai Cloud
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

// +name:"Http"
// +weight:"200"
type _hugoHTTP interface{} //nolint:deadcode,unused

// +docName:"Http plugin for Fluentd"
/*
Sends logs to HTTP/HTTPS endpoints. For details, see [https://docs.fluentd.org/output/http](https://docs.fluentd.org/output/http).

## Example output configurations

```yaml
spec:
  http:
    endpoint: http://logserver.com:9000/api
    buffer:
      tags: "[]"
      flush_interval: 10s
```
*/
type _docHTTP interface{} //nolint:deadcode,unused

// +name:"Http"
// +url:"https://docs.fluentd.org/output/http"
// +version:"more info"
// +description:"Sends logs to HTTP/HTTPS endpoints."
// +status:"GA"
type _metaHTTP interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type HTTPOutputConfig struct {
	// Endpoint for HTTP request.
	Endpoint string `json:"endpoint"`
	// Method for HTTP request. [post, put] (default: post)
	HTTPMethod string `json:"http_method,omitempty"`
	// Proxy for HTTP request.
	Proxy string `json:"proxy,omitempty"`
	// Content-Profile for HTTP request.
	ContentType string `json:"content_type,omitempty"`
	// Using array format of JSON. This parameter is used and valid only for json format. When json_array as true, Content-Profile should be application/json and be able to use JSON data for the HTTP request body.  (default: false)
	JsonArray bool `json:"json_array,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// Additional headers for HTTP request.
	Headers map[string]string `json:"headers,omitempty"`
	// Connection open timeout in seconds.
	OpenTimeout int `json:"open_timeout,omitempty"`
	// Read timeout in seconds.
	ReadTimeout int `json:"read_timeout,omitempty"`
	// TLS timeout in seconds.
	SSLTimeout int `json:"ssl_timeout,omitempty"`
	// The default version of TLS transport. [TLSv1_1, TLSv1_2] (default: TLSv1_2)
	TlsVersion string `json:"tls_version,omitempty"`
	// The cipher configuration of TLS transport. (default: ALL:!aNULL:!eNULL:!SSLv2)
	TlsCiphers string `json:"tls_ciphers,omitempty"`
	// The CA certificate path for TLS.
	TlsCACertPath *secret.Secret `json:"tls_ca_cert_path,omitempty"`
	// The client certificate path for TLS.
	TlsClientCertPath *secret.Secret `json:"tls_client_cert_path,omitempty"`
	// The client private key path for TLS.
	TlsPrivateKeyPath *secret.Secret `json:"tls_private_key_path,omitempty"`
	// The client private key passphrase for TLS.
	TlsPrivateKeyPassphrase *secret.Secret `json:"tls_private_key_passphrase,omitempty"`
	// The verify mode of TLS. [peer, none] (default: peer)
	TlsVerifyMode string `json:"tls_verify_mode,omitempty"`
	// Raise UnrecoverableError when the response code is non success, 1xx/3xx/4xx/5xx. If false, the plugin logs error message instead of raising UnrecoverableError. (default: true)
	ErrorResponseAsUnrecoverable *bool `json:"error_response_as_unrecoverable,omitempty"`
	// List of retryable response codes. If the response code is included in this list, the plugin retries the buffer flush. Since Fluentd v2 the Status code 503 is going to be removed from default. (default: [503])
	RetryableResponseCodes []int `json:"retryable_response_codes,omitempty"`
	// +docLink:"HTTP auth,#http-auth-config"
	Auth *HTTPAuth `json:"auth,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (c *HTTPOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "http"
	http := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(c); err != nil {
		return nil, err
	} else {
		http.Params = params
	}

	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		http.SubDirectives = append(http.SubDirectives, buffer)
	}
	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			http.SubDirectives = append(http.SubDirectives, format)
		}
	}
	if c.Auth != nil {
		if auth, err := c.Auth.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			http.SubDirectives = append(http.SubDirectives, auth)
		}
	}
	return http, nil
}

// +kubebuilder:object:generate=true
// +docName:"HTTP auth config"
// http_auth
type HTTPAuth struct {
	// Username for basic authentication.
	// +docLink:"Secret,../secret/"
	Username *secret.Secret `json:"username"`
	// Password for basic authentication.
	// +docLink:"Secret,../secret/"
	Password *secret.Secret `json:"password"`
}

func (h *HTTPAuth) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "auth",
	}, h, secretLoader)
}

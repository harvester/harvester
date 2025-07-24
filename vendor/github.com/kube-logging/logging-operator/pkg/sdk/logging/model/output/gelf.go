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

package output

import (
	"github.com/cisco-open/operator-tools/pkg/secret"

	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Gelf"
// +weight:"200"
type _hugoGelf interface{} //nolint:deadcode,unused

// +docName:"Gelf output plugin for Fluentd"
/*
For details, see [https://github.com/bmichalkiewicz/fluent-plugin-gelf-best](https://github.com/bmichalkiewicz/fluent-plugin-gelf-best).

## Example
```yaml
spec:
  gelf:
    host: gelf-host
    port: 12201
```
*/
type _docGelf interface{} //nolint:deadcode,unused

// +name:"Gelf"
// +url:"https://github.com/bmichalkiewicz/fluent-plugin-gelf-best"
// +version:"1.3.4"
// +description:"Output plugin writes logs to Graylog"
// +status:"Testing"
type _metaGelf interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type GelfOutputConfig struct {
	// Destination host
	Host string `json:"host"`
	// Destination host port
	Port int `json:"port"`
	// Transport Protocol (default: "udp")
	Protocol string `json:"protocol,omitempty"`
	// Enable TLS (default: false)
	TLS *bool `json:"tls,omitempty"`
	// TLS Options.
	// For details, see [https://github.com/graylog-labs/gelf-rb/blob/72916932b789f7a6768c3cdd6ab69a3c942dbcef/lib/gelf/transport/tcp_tls.rb#L7-L12](https://github.com/graylog-labs/gelf-rb/blob/72916932b789f7a6768c3cdd6ab69a3c942dbcef/lib/gelf/transport/tcp_tls.rb#L7-L12). (default: {})
	TLSOptions map[string]string `json:"tls_options,omitempty"`
	// MaxBytes specifies the maximum size, in bytes, of each individual log message.
	// For details, see [https://github.com/Graylog2/graylog2-server/issues/873](https://github.com/Graylog2/graylog2-server/issues/873)
	// Available since ghcr.io/kube-logging/fluentd:v1.16-4.10-full (default: 3200)
	MaxBytes int `json:"max_bytes,omitempty"`
	// UdpTransportType specifies the UDP chunk size by choosing either WAN or LAN mode.
	// The choice between WAN and LAN affects the UDP chunk size depending on whether you are sending logs within your local network (LAN) or over a longer route (e.g., through the internet). Set this option accordingly.
	// For more details, see:
	// [https://github.com/manet-marketing/gelf_redux/blob/9db64353b6672805152c17642ea8ad39eafb5875/lib/gelf/notifier.rb#L22](https://github.com/manet-marketing/gelf_redux/blob/9db64353b6672805152c17642ea8ad39eafb5875/lib/gelf/notifier.rb#L22)
	// Available since ghcr.io/kube-logging/logging-operator/fluentd:5.3.0-full (default: WAN)
	UdpTransportType string `json:"udp_transport_type,omitempty"`
	// Available since ghcr.io/kube-logging/fluentd:v1.16-4.8-full
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
}

func (s *GelfOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	pluginType := "gelf"
	gelf := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(s); err != nil {
		return nil, err
	} else {
		gelf.Params = params
	}
	if s.Buffer == nil {
		s.Buffer = &Buffer{}
	}
	if buffer, err := s.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		gelf.SubDirectives = append(gelf.SubDirectives, buffer)
	}
	return gelf, nil
}

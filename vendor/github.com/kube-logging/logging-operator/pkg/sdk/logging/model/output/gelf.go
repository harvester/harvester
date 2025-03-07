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

// +name:"GELF"
// +weight:"200"
type _hugoGELF interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[GELF Output](https://github.com/hotschedules/fluent-plugin-gelf-hs)"
// Fluentd output plugin for GELF.
type _docGELF interface{} //nolint:deadcode,unused

// +name:"Gelf"
// +url:"https://github.com/hotschedules/fluent-plugin-gelf-hs"
// +version:"1.0.8"
// +description:"Output plugin writes events to GELF"
// +status:"Testing"
type _metaGelf interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type GELFOutputConfig struct {
	// Destination host
	Host string `json:"host"`
	// Destination host port
	Port int `json:"port"`
	// Transport Protocol (default: "udp")
	Protocol string `json:"protocol,omitempty"`
	// Enable TlS (default: false)
	TLS *bool `json:"tls,omitempty"`
	// TLS options (default: {}). For details, see [https://github.com/graylog-labs/gelf-rb/blob/72916932b789f7a6768c3cdd6ab69a3c942dbcef/lib/gelf/transport/tcp_tls.rb#L7-L12](https://github.com/graylog-labs/gelf-rb/blob/72916932b789f7a6768c3cdd6ab69a3c942dbcef/lib/gelf/transport/tcp_tls.rb#L7-L12).
	TLSOptions map[string]string `json:"tls_options,omitempty"`
}

//
/*
## Example `GELF` output configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Output
metadata:
  name: gelf-output-sample
spec:
  gelf:
    host: gelf-host
    port: 12201
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<match **>
	@type gelf
	@id test_gelf
	host gelf-host
	port 12201
</match>
{{</ highlight >}}
*/
type _expGELF interface{} //nolint:deadcode,unused

func (s *GELFOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
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
	return gelf, nil
}

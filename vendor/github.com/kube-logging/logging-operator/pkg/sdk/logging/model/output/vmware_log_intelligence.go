// Copyright © 2024 Kube logging authors
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

// +name:"VMware Log Intelligence"
// +weight:"200"
type _hugoVMwareLogIntelligence interface{} //nolint:deadcode,unused

// +docName:"VMware Log Intelligence output plugin for Fluentd"
/*
For details, see [https://github.com/vmware/fluent-plugin-vmware-log-intelligence](https://github.com/vmware/fluent-plugin-vmware-log-intelligence).
## Example output configurations
```yaml
spec:
  vmwarelogintelligence:
    endpoint_url: https://data.upgrade.symphony-dev.com/le-mans/v1/streams/ingestion-pipeline-stream
    verify_ssl: true
    http_compress: false
    headers:
      content_type: "application/json"
      authorization:
        valueFrom:
          secretKeyRef:
            name: vmware-log-intelligence-token
            key: authorization
      structure: simple
    buffer:
      chunk_limit_records: 300
      flush_interval: 3s
      retry_max_times: 3
```
*/
type _docVMwareLogIntelligence interface{} //nolint:deadcode,unused

// +name:"VMwareLogIntelligence"
// +url:"https://github.com/vmware/fluent-plugin-vmware-log-intelligence/releases/tag/v2.0.8"
// +version:"v2.0.8"
// +description:"Send your logs to VMware Log Intelligence"
// +status:"GA"
type _metaVMwareLogIntelligence interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"VMwareLogIntelligence"
type VMwareLogIntelligenceOutputConfig struct {
	// Log Intelligence endpoint to send logs to https://github.com/vmware/fluent-plugin-vmware-log-intelligence?tab=readme-ov-file#label-endpoint_url
	EndpointURL string `json:"endpoint_url"`
	// Verify SSL (default: true) https://github.com/vmware/fluent-plugin-vmware-log-intelligence?tab=readme-ov-file#label-verify_ssl
	VerifySSL *bool `json:"verify_ssl" plugin:"default:true"`
	// Compress http request https://github.com/vmware/fluent-plugin-vmware-log-intelligence?tab=readme-ov-file#label-http_compress
	HTTPCompress *bool `json:"http_compress,omitempty"`
	// Required headers for sending logs to VMware Log Intelligence https://github.com/vmware/fluent-plugin-vmware-log-intelligence?tab=readme-ov-file#label-3Cheaders-3E
	Headers LogIntelligenceHeaders `json:"headers"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
}

// +kubebuilder:object:generate=true
// +docName:"VMwareLogIntelligenceHeaders"
// headers
// https://github.com/vmware/fluent-plugin-vmware-log-intelligence?tab=readme-ov-file#label-3Cheaders-3E
type LogIntelligenceHeaders struct {
	// Authorization Bearer token for http request to VMware Log Intelligence
	// +docLink:"Secret,../secret/"
	Authorization *secret.Secret `json:"authorization"`
	// Content Type for http request to VMware Log Intelligence
	ContentType string `json:"content_type" plugin:"default:application/json"`
	// Structure for http request to VMware Log Intelligence
	Structure string `json:"structure" plugin:"default:simple"`
}

// LogIntelligenceHeadersOut is used to convert the input LogIntelligenceHeaders to a fluentd
// output that uses the correct key names for the VMware Log Intelligence plugin. This allows the
// Ouput to accept the config is snake_case (as other output plugins do) but output the fluentd
// <headers> config with the proper key names (ie. content_type -> Content-Type)
type LogIntelligenceHeadersOut struct {
	// Authorization Bearer token for http request to VMware Log Intelligence
	Authorization *secret.Secret `json:"Authorization"`
	// Content Type for http request to VMware Log Intelligence
	ContentType string `json:"Content-Type" plugin:"default:application/json"`
	// Structure for http request to VMware Log Intelligence
	Structure string `json:"structure" plugin:"default:simple"`
}

func (l *LogIntelligenceHeadersOut) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "headers",
	}, l, secretLoader)
}

func (v *VMwareLogIntelligenceOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "vmware_log_intelligence"
	vmwli := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}

	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(v); err != nil {
		return nil, err
	} else {
		vmwli.Params = params
	}

	h := LogIntelligenceHeadersOut{
		ContentType:   v.Headers.ContentType,
		Authorization: v.Headers.Authorization,
		Structure:     v.Headers.Structure,
	}

	if headers, err := h.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		vmwli.SubDirectives = append(vmwli.SubDirectives, headers)
	}

	if v.Buffer == nil {
		v.Buffer = &Buffer{}
	}
	if buffer, err := v.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		vmwli.SubDirectives = append(vmwli.SubDirectives, buffer)
	}

	if v.Format != nil {
		if format, err := v.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			vmwli.SubDirectives = append(vmwli.SubDirectives, format)
		}
	}
	return vmwli, nil
}

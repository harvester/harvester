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
	"errors"

	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"NewRelic"
// +weight:"200"
type _hugoNewRelic interface{} //nolint:deadcode,unused

// +docName:"New Relic Logs plugin for Fluentd"
// **newrelic** output plugin send log data to New Relic Logs
//
// ## Example output configurations
// ```yaml
// spec:
//
//	newrelic:
//	  license_key:
//	    valueFrom:
//	      secretKeyRef:
//	        name: logging-newrelic
//	        key: licenseKey
//
// ```
type _docNewRelic interface{} //nolint:deadcode,unused

// +name:"NewRelic Logs"
// +url:"https://github.com/newrelic/newrelic-fluentd-output"
// +version:"1.2.1"
// +description:"Send logs to New Relic Logs"
// +status:"GA"
type _metaNewRelic interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type NewRelicOutputConfig struct {
	// New Relic API Insert key
	// +docLink:"Secret,../secret/"
	APIKey *secret.Secret `json:"api_key,omitempty"`
	// New Relic License Key (recommended)
	// +docLink:"Secret,../secret/"
	// LicenseKey *secret.Secret `json:"license_key,omitempty"`
	LicenseKey *secret.Secret `json:"license_key,omitempty"`
	// New Relic ingestion endpoint
	// +docLink:"Secret,../secret/"
	BaseURI string `json:"base_uri,omitempty" plugin:"default:https://log-api.newrelic.com/log/v1"`
}

func (c *NewRelicOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "newrelic"
	newrelic := &types.OutputPlugin{
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
		newrelic.Params = params
	}
	if err := c.validateKeys(newrelic, secretLoader); err != nil {
		return nil, err
	}
	return newrelic, nil
}

func (c *NewRelicOutputConfig) validateKeys(newrelic *types.OutputPlugin, secretLoader secret.SecretLoader) error {
	if c.APIKey != nil && c.LicenseKey != nil {
		return errors.New("api_key and license_key cannot be set simultaneously")
	}
	if c.APIKey == nil && c.LicenseKey == nil {
		return errors.New("Either api_key or license_key must be configured")
	}
	return nil
}

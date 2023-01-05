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

package filter

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Geo IP"
// +weight:"200"
type _hugoGeoIP interface{} //nolint:deadcode,unused

// +docName:"Fluentd GeoIP filter"
// Fluentd Filter plugin to add information about geographical location of IP addresses with Maxmind GeoIP databases.
// More information at https://github.com/y-ken/fluent-plugin-geoip
type _docGeoIP interface{} //nolint:deadcode,unused

// +name:"Geo IP"
// +url:"https://github.com/y-ken/fluent-plugin-geoip"
// +version:"1.3.2"
// +description:"Fluentd GeoIP filter"
// +status:"GA"
type _metaGeoIP interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type GeoIP struct {
	//Specify one or more geoip lookup field which has ip address (default: host)
	GeoipLookupKeys string `json:"geoip_lookup_keys,omitempty"`
	//Specify optional geoip database (using bundled GeoLiteCity databse by default)
	GeoipDatabase string `json:"geoip_database,omitempty"`
	//Specify optional geoip2 database (using bundled GeoLite2-City.mmdb by default)
	Geoip2Database string `json:"geoip_2_database,omitempty"`
	//Specify backend library (geoip2_c, geoip, geoip2_compat)
	BackendLibrary string `json:"backend_library,omitempty"`
	// To avoid get stacktrace error with `[null, null]` array for elasticsearch.
	SkipAddingNullRecord bool `json:"skip_adding_null_record,omitempty" plugin:"default:true"`
	// Records are represented as maps: `key: value`
	Records []Record `json:"records,omitempty"`
}

// ## Example `GeoIP` filter configurations
// ```yaml
// apiVersion: logging.banzaicloud.io/v1beta1
// kind: Flow
// metadata:
//
//	name: demo-flow
//
// spec:
//
//	filters:
//	  - geoip:
//	      geoip_lookup_keys: remote_addr
//	      records:
//	        - city: ${city.names.en["remote_addr"]}
//	          location_array: '''[${location.longitude["remote"]},${location.latitude["remote"]}]'''
//	          country: ${country.iso_code["remote_addr"]}
//	          country_name: ${country.names.en["remote_addr"]}
//	          postal_code:  ${postal.code["remote_addr"]}
//	selectors: {}
//	localOutputRefs:
//	  - demo-output
//
// ```
//
// #### Fluentd Config Result
// ```yaml
// <filter **>
//
//	@type geoip
//	@id test_geoip
//	geoip_lookup_keys remote_addr
//	skip_adding_null_record true
//	<record>
//	  city ${city.names.en["remote_addr"]}
//	  country ${country.iso_code["remote_addr"]}
//	  country_name ${country.names.en["remote_addr"]}
//	  location_array '[${location.longitude["remote"]},${location.latitude["remote"]}]'
//	  postal_code ${postal.code["remote_addr"]}
//	</record>
//
// </filter>
// ```
type _expGeoIP interface{} //nolint:deadcode,unused

func (g *GeoIP) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "geoip"
	geoIP := &types.GenericDirective{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "filter",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(g); err != nil {
		return nil, err
	} else {
		geoIP.Params = params
	}
	for _, record := range g.Records {
		if meta, err := record.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			geoIP.SubDirectives = append(geoIP.SubDirectives, meta)
		}
	}
	return geoIP, nil
}

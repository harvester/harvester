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
	"github.com/banzaicloud/operator-tools/pkg/secret"

	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Redis"
// +weight:"200"
type _hugoRedis interface{} //nolint:deadcode,unused

// +docName:"Redis plugin for Fluentd"
// Sends logs to Redis endpoints.
// More info at https://github.com/fluent-plugins-nursery/fluent-plugin-redis
//
// ## Example output configurations
// ```yaml
// spec:
//
//	redis:
//	  host: redis-master.prod.svc.cluster.local
//	  buffer:
//	    tags: "[]"
//	    flush_interval: 10s
//
// ```
type _docRedis interface{} //nolint:deadcode,unused

// +name:"Redis"
// +url:"https://github.com/fluent-plugins-nursery/fluent-plugin-redis"
// +version:"0.3.5"
// +description:"Sends logs to Redis endpoints."
// +status:"GA"
type _metaRedis interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type RedisOutputConfig struct {
	// Host Redis endpoint (default: localhost)
	Host string `json:"host,omitempty"`
	// Port of the Redis server (default: 6379)
	Port int `json:"port,omitempty"`
	// DbNumber database number is optional. (default: 0)
	DbNumber int `json:"db_number,omitempty"`
	// Redis Server password
	Password *secret.Secret `json:"password,omitempty"`
	// insert_key_prefix (default: "${tag}")
	InsertKeyPrefix string `json:"insert_key_prefix,omitempty"`
	// strftime_format Users can set strftime format. (default: "%s")
	StrftimeFormat string `json:"strftime_format,omitempty"`
	// allow_duplicate_key Allow insert key duplicate. It will work as update values. (default: false)
	AllowDuplicateKey bool `json:"allow_duplicate_key,omitempty"`
	// ttl If 0 or negative value is set, ttl is not set in each key.
	TTL int `json:"ttl,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (c *RedisOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "redis"
	redis := &types.OutputPlugin{
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
		redis.Params = params
	}
	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		redis.SubDirectives = append(redis.SubDirectives, buffer)
	}
	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			redis.SubDirectives = append(redis.SubDirectives, format)
		}
	}
	return redis, nil
}

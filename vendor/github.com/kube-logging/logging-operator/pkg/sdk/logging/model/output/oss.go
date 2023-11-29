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
	"github.com/cisco-open/operator-tools/pkg/secret"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/types"
)

// +name:"Alibaba Cloud"
// +weight:"200"
type _hugoOss interface{} //nolint:deadcode,unused

// +docName:"Aliyun OSS plugin for Fluentd"
// **Fluent OSS output plugin** buffers event logs in local files and uploads them to OSS periodically in background threads.
//
// This plugin splits events by using the timestamp of event logs. For example,  a log '2019-04-09 message Hello' is reached, and then another log '2019-04-10 message World' is reached in this order, the former is stored in "20190409.gz" file, and latter in "20190410.gz" file.
//
// **Fluent OSS input plugin** reads data from OSS periodically.
//
// This plugin uses MNS on the same region of the OSS bucket. We must setup MNS and OSS event notification before using this plugin.
//
// [This document](https://help.aliyun.com/document_detail/52656.html) shows how to setup MNS and OSS event notification.
//
// This plugin will poll events from MNS queue and extract object keys from these events, and then will read those objects from OSS.
// More info at https://github.com/aliyun/fluent-plugin-oss
type _docOss interface{} //nolint:deadcode,unused

// +name:"Alibaba Cloud Storage"
// +url:"https://github.com/aliyun/fluent-plugin-oss"
// +version:"0.0.2"
// +description:"Store logs the Alibaba Cloud Object Storage Service"
// +status:"GA"
type _metaOSS interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type OSSOutput struct {
	// OSS endpoint to connect to'
	Endpoint string `json:"endpoint"`
	// Your bucket name
	Bucket string `json:"bucket"`
	// Your access key id
	// +docLink:"Secret,../secret/"
	AccessKeyId *secret.Secret `json:"access_key_id"`
	// Your access secret key
	// +docLink:"Secret,../secret/"
	AaccessKeySecret *secret.Secret `json:"aaccess_key_secret"`
	// Path prefix of the files on OSS (default: fluent/logs)
	Path string `json:"path,omitempty"`
	// Upload crc enabled (default: true)
	UploadCrcEnable bool `json:"upload_crc_enable,omitempty"`
	// Download crc enabled (default: true)
	DownloadCrcEnable bool `json:"download_crc_enable,omitempty"`
	// Timeout for open connections (default: 10)
	OpenTimeout int `json:"open_timeout,omitempty"`
	// Timeout for read response (default: 120)
	ReadTimeout int `json:"read_timeout,omitempty"`
	// OSS SDK log directory (default: /var/log/td-agent)
	OssSdkLogDir string `json:"oss_sdk_log_dir,omitempty"`
	// The format of OSS object keys (default: %{path}/%{time_slice}_%{index}_%{thread_id}.%{file_extension})
	KeyFormat string `json:"key_format,omitempty"`
	// Archive format on OSS: gzip, json, text, lzo, lzma2 (default: gzip)
	StoreAs string `json:"store_as,omitempty"`
	// desc 'Create OSS bucket if it does not exists (default: false)
	AutoCreateBucket bool `json:"auto_create_bucket,omitempty"`
	// Overwrite already existing path (default: false)
	Overwrite bool `json:"overwrite,omitempty"`
	// Check bucket if exists or not (default: true)
	CheckBucket bool `json:"check_bucket,omitempty"`
	// Check object before creation (default: true)
	CheckObject bool `json:"check_object,omitempty"`
	// The length of `%{hex_random}` placeholder(4-16) (default: 4)
	HexRandomLength int `json:"hex_random_length,omitempty"`
	// `sprintf` format for `%{index}` (default: %d)
	IndexFormat string `json:"index_format,omitempty"`
	// Given a threshold to treat events as delay, output warning logs if delayed events were put into OSS
	WarnForDelay string `json:"warn_for_delay,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (o *OSSOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "oss"
	oss := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(o); err != nil {
		return nil, err
	} else {
		oss.Params = params
	}
	if o.Buffer == nil {
		o.Buffer = &Buffer{}
	}
	if buffer, err := o.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		oss.SubDirectives = append(oss.SubDirectives, buffer)
	}

	if o.Format != nil {
		if format, err := o.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			oss.SubDirectives = append(oss.SubDirectives, format)
		}
	}
	return oss, nil
}

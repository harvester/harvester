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

// +name:"Azure Storage"
// +weight:"200"
type _hugoAzure interface{} //nolint:deadcode,unused

// +docName:"Azure Storage output plugin for Fluentd"
// Azure Storage output plugin buffers logs in local file and upload them to Azure Storage periodically.
// More info at https://github.com/microsoft/fluent-plugin-azure-storage-append-blob
type _docAzure interface{} //nolint:deadcode,unused

// +name:"Azure Storage"
// +url:"https://github.com/microsoft/fluent-plugin-azure-storage-append-blob"
// +version:"0.2.1"
// +description:"Store logs in Azure Storage"
// +status:"GA"
type _metaAzure interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"Output Config"
type AzureStorage struct {
	// Path prefix of the files on Azure
	Path string `json:"path,omitempty"`
	// Your azure storage account
	// +docLink:"Secret,../secret/"
	AzureStorageAccount *secret.Secret `json:"azure_storage_account"`
	// Your azure storage access key
	// +docLink:"Secret,../secret/"
	AzureStorageAccessKey *secret.Secret `json:"azure_storage_access_key,omitempty"`
	// Your azure storage sas token
	// +docLink:"Secret,../secret/"
	AzureStorageSasToken *secret.Secret `json:"azure_storage_sas_token,omitempty"`
	// Your azure storage container
	AzureContainer string `json:"azure_container"`
	// Azure Instance Metadata Service API Version
	AzureImdsApiVersion string `json:"azure_imds_api_version,omitempty"`
	// Object key format (default: %{path}%{time_slice}_%{index}.%{file_extension})
	AzureObjectKeyFormat string `json:"azure_object_key_format,omitempty"`
	// Automatically create container if not exists(default: true)
	AutoCreateContainer bool `json:"auto_create_container,omitempty"`
	// Compat format type: out_file, json, ltsv (default: out_file)
	Format string `json:"format,omitempty" plugin:"default:json"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (a *AzureStorage) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "azure-storage-append-blob"
	azure := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(a); err != nil {
		return nil, err
	} else {
		azure.Params = params
	}
	if a.Buffer == nil {
		a.Buffer = &Buffer{}
	}
	if buffer, err := a.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		azure.SubDirectives = append(azure.SubDirectives, buffer)
	}
	return azure, nil
}

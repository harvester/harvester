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

// +name:"File"
// +weight:"200"
type _hugoFile interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// +docName:"[File Output](https://docs.fluentd.org/output/file)"
// This plugin has been designed to output logs or metrics to File.
type _docFile interface{} //nolint:deadcode,unused

// +name:"File"
// +url:"https://docs.fluentd.org/output/file"
// +version:"more info"
// +description:"Output plugin writes events to files"
// +status:"GA"
type _metaFile interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type FileOutputConfig struct {
	// The Path of the file. The actual path is path + time + ".log" by default.
	Path string `json:"path"`
	// The flushed chunk is appended to existence file or not. The default is not appended.
	Append bool `json:"append,omitempty"`
	// +kubebuilder:validation:Optional
	// Add path suffix(default: true)
	AddPathSuffix *bool `json:"add_path_suffix,omitempty" plugin:"default:true"`
	// The suffix of output result.(default: ".log")
	PathSuffix string `json:"path_suffix,omitempty"`
	// Create symlink to temporary buffered file when buffer_type is file. This is useful for tailing file content to check logs.(default: false)
	SymlinkPath bool `json:"symlink_path,omitempty"`
	// Compresses flushed files using gzip. No compression is performed by default.
	Compress string `json:"compress,omitempty"`
	// Performs compression again even if the buffer chunk is already compressed. (default: false)
	Recompress bool `json:"recompress,omitempty"`
	// +docLink:"Format,../format/"
	Format *Format `json:"format,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

//
/*
## Example `File` output configurations

{{< highlight yaml >}}
apiVersion: logging.banzaicloud.io/v1beta1
kind: Output
metadata:
  name: demo-output

spec:
  file:
    path: /tmp/logs/${tag}/%Y/%m/%d.%H.%M
    append: true
    buffer:
      timekey: 1m
      timekey_wait: 10s
      timekey_use_utc: true
{{</ highlight >}}

Fluentd config result:

{{< highlight xml >}}
<match **>
	@type file
	@id test_file
	add_path_suffix true
	append true
	path /tmp/logs/${tag}/%Y/%m/%d.%H.%M
	<buffer tag,time>
	  @type file
	  path /buffers/test_file.*.buffer
	  retry_forever true
	  timekey 1m
	  timekey_use_utc true
	  timekey_wait 30s
	</buffer>
</match>
{{</ highlight >}}
*/
type _expFile interface{} //nolint:deadcode,unused

func (c *FileOutputConfig) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "file"
	file := &types.OutputPlugin{
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
		file.Params = params
	}
	if c.Buffer == nil {
		c.Buffer = &Buffer{}
	}
	if buffer, err := c.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		file.SubDirectives = append(file.SubDirectives, buffer)
	}
	if c.Format != nil {
		if format, err := c.Format.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			file.SubDirectives = append(file.SubDirectives, format)
		}
	}
	return file, nil
}

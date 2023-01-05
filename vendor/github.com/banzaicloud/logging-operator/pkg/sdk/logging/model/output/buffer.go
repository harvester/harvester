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
	"fmt"

	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Buffer"
// +weight:"200"
type _hugoBuffer interface{} //nolint:deadcode,unused

// +name:"Buffer"
// +url:"https://docs.fluentd.org/configuration/buffer-section"
// +version:"mode info"
// +description:"Fluentd event buffer"
// +status:"GA"
type _metaBuffer interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true

type Buffer struct {
	// Disable buffer section (default: false)
	Disabled bool `json:"disabled,omitempty" plugin:"default:false,hidden"`
	// Fluentd core bundles memory and file plugins. 3rd party plugins are also available when installed.
	Type string `json:"type,omitempty"`
	// When tag is specified as buffer chunk key, output plugin writes events into chunks separately per tags. (default: tag,time)
	Tags *string `json:"tags,omitempty"`
	// The path where buffer chunks are stored. The '*' is replaced with random characters. It's highly recommended to leave this default. (default: operator generated)
	Path string `json:"path,omitempty"`
	// The max size of each chunks: events will be written into chunks until the size of chunks become this size (default: 8MB)
	ChunkLimitSize string `json:"chunk_limit_size,omitempty" plugin:"default:8MB"`
	// The max number of events that each chunks can store in it
	ChunkLimitRecords int `json:"chunk_limit_records,omitempty"`
	// The size limitation of this buffer plugin instance. Once the total size of stored buffer reached this threshold, all append operations will fail with error (and data will be lost)
	TotalLimitSize string `json:"total_limit_size,omitempty"`
	//The queue length limitation of this buffer plugin instance
	QueueLimitLength int `json:"queue_limit_length,omitempty"`
	// The percentage of chunk size threshold for flushing. output plugin will flush the chunk when actual size reaches chunk_limit_size * chunk_full_threshold (== 8MB * 0.95 in default)
	ChunkFullThreshold string `json:"chunk_full_threshold,omitempty"`
	//Limit the number of queued chunks. If you set smaller flush_interval, e.g. 1s, there are lots of small queued chunks in buffer. This is not good with file buffer because it consumes lots of fd resources when output destination has a problem. This parameter mitigates such situations.
	QueuedChunksLimitSize int `json:"queued_chunks_limit_size,omitempty"`
	// If you set this option to gzip, you can get Fluentd to compress data records before writing to buffer chunks.
	Compress string `json:"compress,omitempty"`
	// The value to specify to flush/write all buffer chunks at shutdown, or not
	FlushAtShutdown bool `json:"flush_at_shutdown,omitempty"`
	// Default: default (equals to lazy if time is specified as chunk key, interval otherwise)
	// lazy: flush/write chunks once per timekey
	// interval: flush/write chunks per specified time via flush_interval
	// immediate: flush/write chunks immediately after events are appended into chunks
	FlushMode string `json:"flush_mode,omitempty"`
	// Default: 60s
	FlushInterval string `json:"flush_interval,omitempty"`
	// The number of threads of output plugins, which is used to write chunks in parallel
	FlushThreadCount int `json:"flush_thread_count,omitempty"`
	// The sleep interval seconds of threads to wait next flush trial (when no chunks are waiting)
	FlushThreadInterval string `json:"flush_thread_interval,omitempty"`
	// The sleep interval seconds of threads between flushes when output plugin flushes waiting chunks next to next
	FlushThreadBurstInterval string `json:"flush_thread_burst_interval,omitempty"`
	// The timeout seconds until output plugin decides that async write operation fails
	DelayedCommitTimeout string `json:"delayed_commit_timeout,omitempty"`
	// How output plugin behaves when its buffer queue is full
	// throw_exception: raise exception to show this error in log
	// block: block processing of input plugin to emit events into that buffer
	// drop_oldest_chunk: drop/purge oldest chunk to accept newly incoming chunk
	OverflowAction string `json:"overflow_action,omitempty"`
	// The maximum seconds to retry to flush while failing, until plugin discards buffer chunks
	RetryTimeout string `json:"retry_timeout,omitempty"`
	// If true, plugin will ignore retry_timeout and retry_max_times options and retry flushing forever
	RetryForever *bool `json:"retry_forever,omitempty" plugin:"default:true"`
	// The maximum number of times to retry to flush while failing
	RetryMaxTimes int `json:"retry_max_times,omitempty"`
	// The ratio of retry_timeout to switch to use secondary while failing (Maximum valid value is 1.0)
	RetrySecondaryThreshold string `json:"retry_secondary_threshold,omitempty"`
	// exponential_backoff: wait seconds will become large exponentially per failures
	// periodic: output plugin will retry periodically with fixed intervals (configured via retry_wait)
	RetryType string `json:"retry_type,omitempty"`
	// Seconds to wait before next retry to flush, or constant factor of exponential backoff
	RetryWait string `json:"retry_wait,omitempty"`
	// The base number of exponential backoff for retries
	RetryExponentialBackoffBase string `json:"retry_exponential_backoff_base,omitempty"`
	// The maximum interval seconds for exponential backoff between retries while failing
	RetryMaxInterval string `json:"retry_max_interval,omitempty"`
	// If true, output plugin will retry after randomized interval not to do burst retries
	RetryRandomize bool `json:"retry_randomize,omitempty"`
	// Instead of storing unrecoverable chunks in the backup directory, just discard them. This option is new in Fluentd v1.2.6.
	DisableChunkBackup bool `json:"disable_chunk_backup,omitempty"`
	// Output plugin will flush chunks per specified time (enabled when time is specified in chunk keys)
	// +kubebuilder:validation:Optional
	Timekey string `json:"timekey" plugin:"default:10m"`
	// Output plugin writes chunks after timekey_wait seconds later after timekey expiration
	TimekeyWait string `json:"timekey_wait,omitempty" plugin:"default:1m"`
	// Output plugin decides to use UTC or not to format placeholders using timekey
	TimekeyUseUtc bool `json:"timekey_use_utc,omitempty"`
	// The timezone (-0700 or Asia/Tokyo) string for formatting timekey placeholders
	TimekeyZone string `json:"timekey_zone,omitempty"`
}

func (b *Buffer) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	if b.Disabled {
		return nil, nil
	}

	metadata := types.PluginMeta{
		Type:      "file",
		Directive: "buffer",
	}
	buffer := b.DeepCopy()
	// Set type
	if buffer.Type != "" {
		metadata.Type = buffer.Type
	}
	// Set default values for tags
	if buffer.Tags != nil {
		metadata.Tag = *buffer.Tags
	} else {
		metadata.Tag = "tag,time"
	}
	if buffer.Path == "" && buffer.Type != "memory" {
		buffer.Path = fmt.Sprintf("/buffers/%s.*.buffer", id)
	}
	// Cleanup non parameter configurations
	buffer.Type = ""
	buffer.Tags = nil
	return types.NewFlatDirective(metadata, buffer, secretLoader)
}

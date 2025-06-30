// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
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

// +name:"Disk buffer"
// +weight:"200"
type _hugoDiskBuffer interface{} //nolint:deadcode,unused

// +docName:"Disk buffer configuration"
// The parameters of the syslog-ng disk buffer. Using a disk buffer on the output helps avoid message loss in case of a system failure on the destination side.
// For details on how [syslog-ng disk buffers work, see the documentation of the AxoSyslog syslog-ng distribution](https://axoflow.com/docs/axosyslog-core/chapter-routing-filters/concepts-diskbuffer/).
type _docDiskBuffer interface{} //nolint:deadcode,unused

// +name:"disk-buffer configuration"
// +url:"https://axoflow.com/docs/axosyslog-core/chapter-routing-filters/concepts-diskbuffer/"
// +description:"disk-buffer configuration"
// +status:"Testing"
type _metaDiskBuffer interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// Documentation: https://axoflow.com/docs/axosyslog-core/chapter-routing-filters/concepts-diskbuffer/
type DiskBuffer struct {
	// This is a required option. The maximum size of the disk-buffer in bytes. The minimum value is 1048576 bytes.
	DiskBufSize int64 `json:"disk_buf_size"`
	//  If set to yes, syslog-ng OSE cannot lose logs in case of reload/restart, unreachable destination or syslog-ng OSE crash. This solution provides a slower, but reliable disk-buffer option.
	Reliable bool `json:"reliable"`
	// Prunes the unused space in the LogMessage representation
	Compaction *bool `json:"compaction,omitempty"`
	// Description: Defines the folder where the disk-buffer files are stored.
	Dir string `json:"dir,omitempty"`
	// Use this option if the option reliable() is set to no. This option contains the number of messages stored in overflow queue.
	MemBufLength *int64 `json:"mem_buf_length,omitempty"`
	// Use this option if the option reliable() is set to yes. This option contains the size of the messages in bytes that is used in the memory part of the disk buffer.
	MemBufSize *int64 `json:"mem_buf_size,omitempty"`
	// The number of messages stored in the output buffer of the destination.
	QOutSize *int64 `json:"q_out_size,omitempty"`
}

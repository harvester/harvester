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

// +name:"File"
// +weight:"200"
type _hugoFile interface{} //nolint:deadcode,unused

// +docName:"File output plugin for syslog-ng"
// The `file` output stores log records in a plain text file.
//
// {{< highlight yaml >}}
//
//	spec:
//	  file:
//	    path: /mnt/archive/logs/${YEAR}/${MONTH}/${DAY}/app.log
//	    create_dirs: true
//
// {{</ highlight >}}
//
// For details on the available options of the output, see the [syslog-ng documentation](https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/36#TOPIC-1829044).
type _docFile interface{} //nolint:deadcode,unused

// +name:"File"
// +url:"https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.17/administration-guide/32"
// +description:"SStoring messages in plain-text files"
// +status:"Testing"
type _metaFile interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
// Documentation: https://www.syslog-ng.com/technical-documents/doc/syslog-ng-open-source-edition/3.37/administration-guide/36#TOPIC-1829044
type FileOutput struct {
	// Store file path
	Path string `json:"path" syslog-ng:"pos=0"`
	// Enable creating non-existing directories. (default: false)
	CreateDirs bool `json:"create_dirs,omitempty"`
	// The group of the directories created by syslog-ng. To preserve the original properties of an existing directory, use the option without specifying an attribute: dir-group(). (default: Use the global settings)
	DirGroup string `json:"dir_group,omitempty"`
	// The owner of the directories created by syslog-ng. To preserve the original properties of an existing directory, use the option without specifying an attribute: dir-owner(). (default: Use the global settings)
	DirOwner string `json:"dir_owner,omitempty"`
	// The permission mask of directories created by syslog-ng. Log directories are only created if a file after macro expansion refers to a non-existing directory, and directory creation is enabled (see also the create-dirs() option). For octal numbers prefix the number with 0, for example use 0755 for rwxr-xr-x.(default: Use the global settings)
	DirPerm int `json:"dir_perm,omitempty"`
	// This option enables putting outgoing messages into the disk buffer of the destination to avoid message loss in case of a system failure on the destination side. For details, see the [Syslog-ng DiskBuffer options](../disk_buffer/). (default: false)
	DiskBuffer  *DiskBuffer `json:"disk_buffer,omitempty"`
	Template    string      `json:"template,omitempty"`
	PersistName string      `json:"persist_name,omitempty"`
}

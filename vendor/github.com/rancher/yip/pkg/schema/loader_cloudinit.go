//   Copyright 2020 Ettore Di Giacinto <mudler@mocaccino.org>
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package schema

import (
	"fmt"
	"strconv"

	"github.com/twpayne/go-vfs/v4"

	cloudconfig "github.com/rancher/yip/pkg/schema/cloudinit"
)

type cloudInit struct{}

// Load transpiles a cloud-init style
// file ( https://cloudinit.readthedocs.io/en/latest/topics/examples.html)
// to a yip schema.
// As Yip supports multi-stages, it is encoded in the supplied one.
// fs is used to parse the user data required from /etc/passwd.
func (cloudInit) Load(s []byte, fs vfs.FS) (*YipConfig, error) {
	cc, err := cloudconfig.NewCloudConfig(string(s))
	if err != nil {
		return nil, err
	}

	// Decode users and SSH Keys
	sshKeys := make(map[string][]string)
	users := make(map[string]User)
	userstoKey := []string{}

	for _, u := range cc.Users {
		userstoKey = append(userstoKey, u.Name)
		users[u.Name] = User{
			Name:         u.Name,
			PasswordHash: u.PasswordHash,
			GECOS:        u.GECOS,
			Homedir:      u.Homedir,
			NoCreateHome: u.NoCreateHome,
			PrimaryGroup: u.PrimaryGroup,
			Groups:       u.Groups,
			NoUserGroup:  u.NoUserGroup,
			System:       u.System,
			NoLogInit:    u.NoLogInit,
			Shell:        u.Shell,
			UID:          u.UID,
			LockPasswd:   u.LockPasswd,
		}
		sshKeys[u.Name] = u.SSHAuthorizedKeys
	}

	for _, uu := range userstoKey {
		_, exists := sshKeys[uu]
		if !exists {
			sshKeys[uu] = cc.SSHAuthorizedKeys
		} else {
			sshKeys[uu] = append(sshKeys[uu], cc.SSHAuthorizedKeys...)
		}
	}

	// If no users are defined, then assume global ssh_authorized_keys is assigned to root
	if len(userstoKey) == 0 && len(cc.SSHAuthorizedKeys) > 0 {
		sshKeys["root"] = cc.SSHAuthorizedKeys
	}

	// Decode writeFiles
	var f []File
	for _, ff := range append(cc.WriteFiles, cc.MilpaFiles...) {
		newFile := File{
			Path:        ff.Path,
			OwnerString: ff.Owner,
			Content:     ff.Content,
			Encoding:    ff.Encoding,
		}
		newFile.Permissions, err = parseOctal(ff.RawFilePermissions)
		if err != nil {
			return nil, fmt.Errorf("converting permission %s for %s: %w", ff.RawFilePermissions, ff.Path, err)
		}
		f = append(f, newFile)
	}

	stages := []Stage{{
		Commands: cc.RunCmd,
		Files:    f,
		Users:    users,
		SSHKeys:  sshKeys,
	}}

	for _, d := range cc.Partitioning.Devices {
		layout := &Layout{}
		layout.Expand = &Expand{Size: 0}
		layout.Device = &Device{Path: d}
		stages = append(stages, Stage{Layout: *layout})
	}

	result := &YipConfig{
		Stages: map[string][]Stage{
			"boot": stages,
			"initramfs": {{
				Hostname: cc.Hostname,
			}},
		},
	}

	// optimistically load data as yip yaml
	yipConfig, err := yipYAML{}.Load(s, fs)
	if err == nil {
		for k, v := range yipConfig.Stages {
			result.Stages[k] = append(result.Stages[k], v...)
		}
	}

	return result, nil
}

func parseOctal(srv string) (uint32, error) {
	if srv == "" {
		return 0, nil
	}
	i, err := strconv.ParseUint(srv, 8, 32)
	if err != nil {
		return 0, err
	}
	return uint32(i), nil
}

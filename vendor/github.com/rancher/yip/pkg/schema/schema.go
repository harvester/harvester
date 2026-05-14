//   Copyright 2019 Ettore Di Giacinto <mudler@mocaccino.org>
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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/user"
	"strings"

	"github.com/google/shlex"

	config "github.com/rancher/yip/pkg/schema/cloudinit"

	"github.com/hashicorp/go-multierror"
	"github.com/itchyny/gojq"
	"github.com/twpayne/go-vfs/v4"
	"gopkg.in/yaml.v3"
)

type YipEntity struct {
	Path   string `yaml:"path"`
	Entity string `yaml:"entity"`
}

type File struct {
	Path         string
	Permissions  uint32
	Owner, Group int
	Content      string
	Encoding     string
	OwnerString  string
}

type Download struct {
	Path         string
	URL          string
	Permissions  uint32
	Owner, Group int
	Timeout      int
	OwnerString  string
}

type Directory struct {
	Path         string
	Permissions  uint32
	Owner, Group int
}

type DataSource struct {
	Providers []string `yaml:"providers,omitempty"`
	Path      string   `yaml:"path,omitempty"`
}

type Git struct {
	Auth       Auth   `yaml:"auth,omitempty"`
	URL        string `yaml:"url,omitempty"`
	Path       string `yaml:"path,omitempty"`
	Branch     string `yaml:"branch,omitempty"`
	BranchOnly bool   `yaml:"branch_only,omitempty"`
}

type Auth struct {
	Username   string `yaml:"username,omitempty"`
	Password   string `yaml:"password,omitempty"`
	PrivateKey string `yaml:"private_key,omitempty"`

	Insecure  bool   `yaml:"insecure,omitempty"`
	PublicKey string `yaml:"public_key,omitempty"`
}

type User struct {
	Name              string   `yaml:"name,omitempty"`
	PasswordHash      string   `yaml:"passwd,omitempty"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
	GECOS             string   `yaml:"gecos,omitempty"`
	Homedir           string   `yaml:"homedir,omitempty"`
	NoCreateHome      bool     `yaml:"no_create_home,omitempty"`
	PrimaryGroup      string   `yaml:"primary_group,omitempty"`
	Groups            []string `yaml:"groups,omitempty"`
	NoUserGroup       bool     `yaml:"no_user_group,omitempty"`
	System            bool     `yaml:"system,omitempty"`
	NoLogInit         bool     `yaml:"no_log_init,omitempty"`
	Shell             string   `yaml:"shell,omitempty"`
	LockPasswd        bool     `yaml:"lock_passwd,omitempty"`
	UID               string   `yaml:"uid,omitempty"`
}

func (u User) Exists() bool {
	_, err := user.Lookup(u.Name)
	return err == nil
}

type Layout struct {
	Device *Device     `yaml:"device,omitempty"`
	Expand *Expand     `yaml:"expand_partition,omitempty"`
	Parts  []Partition `yaml:"add_partitions,omitempty"`
}

type Device struct {
	Label string `yaml:"label,omitempty"`
	Path  string `yaml:"path,omitempty"`
}

type Expand struct {
	Size uint `yaml:"size,omitempty"`
}

type Partition struct {
	FSLabel    string `yaml:"fsLabel,omitempty"`
	Size       uint   `yaml:"size,omitempty"`
	PLabel     string `yaml:"pLabel,omitempty"`
	FileSystem string `yaml:"filesystem,omitempty"`
}

type Dependency struct {
	Name string `yaml:"name,omitempty"`
}

type Stage struct {
	Commands    []string    `yaml:"commands,omitempty"`
	Files       []File      `yaml:"files,omitempty"`
	Downloads   []Download  `yaml:"downloads,omitempty"`
	Directories []Directory `yaml:"directories,omitempty"`
	If          string      `yaml:"if,omitempty"`

	EnsureEntities  []YipEntity         `yaml:"ensure_entities,omitempty"`
	DeleteEntities  []YipEntity         `yaml:"delete_entities,omitempty"`
	Dns             DNS                 `yaml:"dns,omitempty"`
	Hostname        string              `yaml:"hostname,omitempty"`
	Name            string              `yaml:"name,omitempty"`
	Sysctl          map[string]string   `yaml:"sysctl,omitempty"`
	SSHKeys         map[string][]string `yaml:"authorized_keys,omitempty"`
	Node            string              `yaml:"node,omitempty"`
	Users           map[string]User     `yaml:"users,omitempty"`
	Systemctl       Systemctl           `yaml:"systemctl,omitempty"`
	Environment     map[string]string   `yaml:"environment,omitempty"`
	EnvironmentFile string              `yaml:"environment_file,omitempty"`

	After []Dependency `yaml:"after,omitempty"`

	DataSources DataSource `yaml:"datasource,omitempty"`
	Layout      Layout     `yaml:"layout,omitempty"`

	SystemdFirstBoot map[string]string `yaml:"systemd_firstboot,omitempty"`

	TimeSyncd map[string]string `yaml:"timesyncd,omitempty"`
	Git       Git               `yaml:"git,omitempty"`
}

type Systemctl struct {
	Enable  []string `yaml:"enable,omitempty"`
	Disable []string `yaml:"disable,omitempty"`
	Start   []string `yaml:"start,omitempty"`
	Mask    []string `yaml:"mask,omitempty"`
}

type DNS struct {
	Nameservers []string `yaml:"nameservers,omitempty"`
	DnsSearch   []string `yaml:"search,omitempty"`
	DnsOptions  []string `yaml:"options,omitempty"`
	Path        string   `yaml:"path,omitempty"`
}

type YipConfig struct {
	Name   string             `yaml:"name,omitempty"`
	Stages map[string][]Stage `yaml:"stages,omitempty"`
}

type Loader func(s string, fs vfs.FS, m Modifier) ([]byte, error)
type Modifier func(s []byte) ([]byte, error)

type yipLoader interface {
	Load([]byte, vfs.FS) (*YipConfig, error)
}

func Load(s string, fs vfs.FS, l Loader, m Modifier) (*YipConfig, error) {
	if m == nil {
		m = func(b []byte) ([]byte, error) { return b, nil }
	}
	if l == nil {
		l = func(c string, fs vfs.FS, m Modifier) ([]byte, error) { return m([]byte(c)) }
	}
	data, err := l(s, fs, m)
	if err != nil {
		return nil, fmt.Errorf("error while loading yipconfig: %s", err.Error())
	}

	loader, err := detect(data)
	if err != nil {
		return nil, fmt.Errorf("invalid file type: %s", err.Error())
	}
	return loader.Load(data, fs)
}

func detect(b []byte) (yipLoader, error) {
	switch {
	case config.IsCloudConfig(string(b)):
		return cloudInit{}, nil

	default:
		return yipYAML{}, nil
	}
}

// FromFile loads a yip config from a YAML file
func FromFile(s string, fs vfs.FS, m Modifier) ([]byte, error) {
	yamlFile, err := fs.ReadFile(s)
	if err != nil {
		return nil, err
	}
	return m(yamlFile)
}

// FromUrl loads a yip config from a url
func FromUrl(s string, fs vfs.FS, m Modifier) ([]byte, error) {
	resp, err := http.Get(s)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	buf := bytes.NewBuffer([]byte{})
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return nil, err
	}
	return m(buf.Bytes())
}

// DotNotationModifier read a byte sequence in dot notation and returns a byte sequence in yaml
// e.g. foo.bar=boo
func DotNotationModifier(s []byte) ([]byte, error) {
	v := stringToMap(string(s))

	data, err := dotToYAML(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func jq(command string, data map[string]interface{}) (map[string]interface{}, error) {
	query, err := gojq.Parse(command)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, err
	}
	iter := code.Run(data)

	v, ok := iter.Next()
	if !ok {
		return nil, errors.New("failed getting result from gojq")
	}
	if err, ok := v.(error); ok {
		return nil, err
	}
	if t, ok := v.(map[string]interface{}); ok {
		return t, nil
	}

	return make(map[string]interface{}), nil
}

func dotToYAML(v map[string]interface{}) ([]byte, error) {
	data := map[string]interface{}{}
	var errs error

	for k, value := range v {
		newData, err := jq(fmt.Sprintf(".%s=\"%s\"", k, value), data)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
		data = newData
	}

	out, err := yaml.Marshal(&data)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	return out, err
}

func stringToMap(s string) map[string]interface{} {
	v := map[string]interface{}{}

	splitted, _ := shlex.Split(s)
	for _, item := range splitted {
		parts := strings.SplitN(item, "=", 2)
		value := "true"
		if len(parts) > 1 {
			value = strings.Trim(parts[1], `"`)
		}
		key := strings.Trim(parts[0], `"`)
		v[key] = value
	}

	return v
}

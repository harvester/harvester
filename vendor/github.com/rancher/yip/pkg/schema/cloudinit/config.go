// This is an adaptation of coreOS/cloudinit.
// https://github.com/coreos/coreos-cloudinit/blob/master/config/config.go

package config

import (
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// NewCloudConfig instantiates a new CloudConfig from the given contents (a
// string of YAML), returning any error encountered. It will ignore unknown
// fields but log encountering them.
func NewCloudConfig(contents string) (*CloudConfig, error) {
	var cfg CloudConfig
	err := yaml.Unmarshal([]byte(contents), &cfg)
	return &cfg, err
}

func IsCloudConfig(userdata string) bool {
	header := strings.SplitN(userdata, "\n", 2)[0]

	// Trim trailing whitespaces
	header = strings.TrimRightFunc(header, unicode.IsSpace)

	return (header == "#cloud-config")
}

// CloudConfig encapsulates the entire cloud-config configuration file and maps
// directly to YAML. Fields that cannot be set in the cloud-config (fields
// used for internal use) have the YAML tag '-' so that they aren't marshalled.
type CloudConfig struct {
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
	WriteFiles        []File   `yaml:"write_files,omitempty"`
	Hostname          string   `yaml:"hostname,omitempty"`
	Users             []User   `yaml:"users,omitempty"`
	RunCmd            []string `yaml:"runcmd,omitempty"`

	Partitioning GrowPart `yaml:"growpart"`
	// this one is legacy, can be removed when no more kip controllers use it
	MilpaFiles []File `yaml:"milpa_files,omitempty"`
	// Todo: add additional parameters supported by traditional cloud-init
}

type GrowPart struct {
	Mode    string   `yaml:"mode,omitempty"`
	Devices []string `yaml:"devices"`
}

type File struct {
	Encoding           string `yaml:"encoding,omitempty" valid:"^(base64|b64|gz|gzip|gz\\+base64|gzip\\+base64|gz\\+b64|gzip\\+b64)$"`
	Content            string `yaml:"content,omitempty"`
	Owner              string `yaml:"owner,omitempty"`
	Path               string `yaml:"path,omitempty"`
	RawFilePermissions string `yaml:"permissions,omitempty" valid:"^0?[0-7]{3,4}$"`
}

type User struct {
	Name                 string   `yaml:"name,omitempty"`
	PasswordHash         string   `yaml:"passwd,omitempty"`
	SSHAuthorizedKeys    []string `yaml:"ssh_authorized_keys,omitempty"`
	SSHImportGithubUser  string   `yaml:"coreos_ssh_import_github,omitempty"       deprecated:"trying to fetch from a remote endpoint introduces too many intermittent errors"`
	SSHImportGithubUsers []string `yaml:"coreos_ssh_import_github_users,omitempty" deprecated:"trying to fetch from a remote endpoint introduces too many intermittent errors"`
	SSHImportURL         string   `yaml:"coreos_ssh_import_url,omitempty"          deprecated:"trying to fetch from a remote endpoint introduces too many intermittent errors"`
	GECOS                string   `yaml:"gecos,omitempty"`
	Homedir              string   `yaml:"homedir,omitempty"`
	NoCreateHome         bool     `yaml:"no_create_home,omitempty"`
	PrimaryGroup         string   `yaml:"primary_group,omitempty"`
	Groups               []string `yaml:"groups,omitempty"`
	NoUserGroup          bool     `yaml:"no_user_group,omitempty"`
	System               bool     `yaml:"system,omitempty"`
	NoLogInit            bool     `yaml:"no_log_init,omitempty"`
	Shell                string   `yaml:"shell,omitempty"`
	LockPasswd           bool     `yaml:"lock_passwd"`
	UID                  string   `yaml:"uid"`
}

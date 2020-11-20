package config

import (
	"fmt"
	"os"
	"strconv"
)

type K3OS struct {
	DataSources    []string          `json:"dataSources,omitempty"`
	Modules        []string          `json:"modules,omitempty"`
	Sysctls        map[string]string `json:"sysctls,omitempty"`
	NTPServers     []string          `json:"ntpServers,omitempty"`
	DNSNameservers []string          `json:"dnsNameservers,omitempty"`
	Wifi           []Wifi            `json:"wifi,omitempty"`
	Password       string            `json:"password,omitempty"`
	ServerURL      string            `json:"serverUrl,omitempty"`
	Token          string            `json:"token,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	K3sArgs        []string          `json:"k3sArgs,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	Taints         []string          `json:"taints,omitempty"`
	Install        *Install          `json:"install,omitempty"`
}

type Wifi struct {
	Name       string `json:"name,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

type Install struct {
	ForceEFI  bool   `json:"forceEfi,omitempty"`
	Device    string `json:"device,omitempty"`
	ConfigURL string `json:"configUrl,omitempty"`
	Silent    bool   `json:"silent,omitempty"`
	ISOURL    string `json:"isoUrl,omitempty"`
	PowerOff  bool   `json:"powerOff,omitempty"`
	NoFormat  bool   `json:"noFormat,omitempty"`
	Debug     bool   `json:"debug,omitempty"`
	TTY       string `json:"tty,omitempty"`
}

type CloudConfig struct {
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
	WriteFiles        []File   `json:"writeFiles,omitempty"`
	Hostname          string   `json:"hostname,omitempty"`
	K3OS              K3OS     `json:"k3os,omitempty"`
	Runcmd            []string `json:"runCmd,omitempty"`
	Bootcmd           []string `json:"bootCmd,omitempty"`
	Initcmd           []string `json:"initCmd,omitempty"`
}

type File struct {
	Encoding           string `json:"encoding"`
	Content            string `json:"content"`
	Owner              string `json:"owner"`
	Path               string `json:"path"`
	RawFilePermissions string `json:"permissions"`
}

func (f *File) Permissions() (os.FileMode, error) {
	if f.RawFilePermissions == "" {
		return os.FileMode(0644), nil
	}
	// parse string representation of file mode as integer
	perm, err := strconv.ParseInt(f.RawFilePermissions, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("unable to parse file permissions %q as integer", f.RawFilePermissions)
	}
	return os.FileMode(perm), nil
}

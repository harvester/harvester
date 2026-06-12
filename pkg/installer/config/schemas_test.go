package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/util"
)

func TestToHarvesterConfig(t *testing.T) {
	testCases := []struct {
		input    []byte
		expected *HarvesterConfig
		err      error
	}{
		{
			input: util.LoadFixture(t, "harvester-config.yaml"),
			expected: &HarvesterConfig{
				SchemeVersion: SchemeVersion,
				ServerURL:     "https://someserver:6443",
				Token:         "TOKEN_VALUE",
				OS: OS{
					SSHAuthorizedKeys: []string{
						"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB...",
						"github:username",
					},
					Hostname: "myhost",
					Modules: []string{
						"kvm",
						"nvme",
					},
					Sysctls: map[string]string{
						"kernel.printk":        "4 4 1 7",
						"kernel.kptr_restrict": "1",
					},
					DNSNameservers: []string{
						"8.8.8.8",
						"1.1.1.1",
					},
					NTPServers: []string{
						"0.us.pool.ntp.org",
						"1.us.pool.ntp.org",
					},
					Password: "rancher",
					Environment: map[string]string{
						"http_proxy":  "http://myserver",
						"https_proxy": "http://myserver",
					},
				},
				Install: Install{
					Mode: "create",
					ManagementInterface: Network{
						Interfaces: []NetworkInterface{{Name: "ens0"}, {Name: "ens3"}},
						Method:     "dhcp",
					},
					ForceEFI: true,
					Device:   "/dev/vda",
					Silent:   true,
					ISOURL:   "http://myserver/test.iso",
					PowerOff: true,
					NoFormat: true,
					Debug:    true,
					TTY:      "ttyS0",
				},
			},
			err: nil,
		},
	}

	for _, testCase := range testCases {
		output, err := LoadHarvesterConfig(testCase.input)
		assert.Equal(t, testCase.expected, output)
		assert.Equal(t, testCase.err, err)
	}
}

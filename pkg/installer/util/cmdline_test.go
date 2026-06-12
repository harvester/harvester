package util

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rancher/mapper/values"
	"github.com/stretchr/testify/assert"
)

func Test_parseCmdLineWithPrefix(t *testing.T) {
	cmdline := `x y harvester.a.b=true "harvester.c=d" harvester.e harvester.f=1 harvester.f=2`
	m, err := ParseCmdLine(cmdline, "harvester")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"a": map[string]interface{}{"b": "true"},
		"c": "d",
		"e": "true",
		"f": []string{"1", "2"},
	}
	assert.Equal(t, want, m)
}

func Test_parseCmdLineWithoutPrefix(t *testing.T) {
	cmdline := `mode=live console=tty1 console=ttyS0,115200n8`
	m, err := ParseCmdLine(cmdline, "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"mode":    "live",
		"console": []string{"tty1", "ttyS0,115200n8"},
	}
	assert.Equal(t, want, m)
}

func Test_parseCmdLineWithNetworkInterface(t *testing.T) {
	type testcase struct {
		cmdline       string
		expectation   []interface{}
		expectedError error
	}

	testcases := []testcase{
		{
			cmdline: `harvester.os.sshAuthorizedKeys=a  harvester.install.management_interface.method=dhcp harvester.install.management_interface.bond_options.mode=balance-tlb harvester.install.management_interface.bond_options.miimon=100 harvester.os.sshAuthorizedKeys=b harvester.install.mode=create harvester.install.management_interface.interfaces="hwAddr: be:44:8c:b0:5d:f2" harvester.install.management_interface.interfaces="hwAddr:   10:fe:71:05:57:fd" harvester.install.management_interface.interfaces="ens3" harvester.install.management_interface.interfaces="name:ens5"`,
			expectation: []interface{}{
				map[string]interface{}{"hwAddr": "be:44:8c:b0:5d:f2"},
				map[string]interface{}{"hwAddr": "10:fe:71:05:57:fd"},
				map[string]interface{}{"name": "ens3"},
				map[string]interface{}{"name": "ens5"},
			},
			expectedError: nil,
		},
		{
			cmdline: `harvester.install.management_interface.interfaces="ens3"`,
			expectation: []interface{}{
				map[string]interface{}{"name": "ens3"},
			},
			expectedError: nil,
		},
		{
			cmdline: `harvester.install.management_interface.interfaces="hwAddr: be:44:8c:b0:5d:f2,ens3" harvester.install.management_interface.interfaces="10:fe:71:05:57:fd,name:ens5"`,
			expectation: []interface{}{
				map[string]interface{}{"hwAddr": "be:44:8c:b0:5d:f2", "name": "ens3"},
				map[string]interface{}{"hwAddr": "10:fe:71:05:57:fd", "name": "ens5"},
			},
			expectedError: nil,
		},
		{
			// LWW (Last Write Wins) behavior for duplicate fields.
			cmdline: `harvester.install.management_interface.interfaces="hwAddr: be:44:8c:b0:5d:f2,ens3,name:ens4" harvester.install.management_interface.interfaces="10:fe:71:05:57:fd,name:ens5,hwAddr:10:fe:71:ff:ff:ff"`,
			expectation: []interface{}{
				map[string]interface{}{"hwAddr": "be:44:8c:b0:5d:f2", "name": "ens4"},
				map[string]interface{}{"hwAddr": "10:fe:71:ff:ff:ff", "name": "ens5"},
			},
			expectedError: nil,
		},
		{
			cmdline:       `harvester.install.management_interface.interfaces="hwAddr: be:4:8c:b0:5d:x2,ens3"`,
			expectation:   []interface{}{},
			expectedError: fmt.Errorf("could not parse interface details: address be:4:8c:b0:5d:x2: invalid MAC address"),
		},
		{
			cmdline:       `harvester.install.management_interface.interfaces=""`,
			expectation:   []interface{}{},
			expectedError: fmt.Errorf("empty interface details"),
		},
		{
			cmdline:       `harvester.os.sshAuthorizedKeys=a  harvester.install.management_interface.method=dhcp harvester.install.management_interface.bond_options.mode=balance-tlb harvester.install.management_interface.bond_options.miimon=100 harvester.os.sshAuthorizedKeys=b harvester.install.mode=create harvester.install.management_interface.interfaces="foo:bar:foobar"`,
			expectation:   []interface{}{},
			expectedError: fmt.Errorf("could not parse interface details"),
		},
	}

	for _, tc := range testcases {
		m, err := ParseCmdLine(tc.cmdline, "harvester")
		if err != nil {
			if tc.expectedError != nil {
				assert.True(t, strings.Contains(err.Error(), tc.expectedError.Error()), "unexpected error")
			} else {
				t.Fatal(err)
			}
		} else {
			want := tc.expectation
			have, ok := values.GetValue(m, "install", "management_interface", "interfaces")
			if !ok {
				t.Fatal(fmt.Errorf("no network interfaces found"))
			}

			assert.Equal(t, want, have)
		}
	}
}

func Test_parseCmdLineWithSchemeVersion(t *testing.T) {
	cmdline := `harvester.os.sshAuthorizedKeys=a  harvester.install.management_interface.method=dhcp harvester.install.management_interface.bond_options.mode=balance-tlb harvester.install.management_interface.bond_options.miimon=100 harvester.os.sshAuthorizedKeys=b harvester.install.mode=create harvester.install.management_interface.interfaces="hwAddr: be:44:8c:b0:5d:f2" harvester.install.management_interface.interfaces="hwAddr:   10:fe:71:05:57:fd" harvester.scheme_version=1`

	m, err := ParseCmdLine(cmdline, "harvester")
	assert.NoError(t, err, "expected no error while parsing arguments")

	val, ok := m["scheme_version"]
	assert.True(t, ok, "expected to find key scheme_version")
	var tmp uint64
	assert.IsType(t, tmp, val, "expected to find scheme_version to be type uint")
}

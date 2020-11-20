package console

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHarvesterManifestContent(t *testing.T) {
	d := map[string]string{
		"a": "b",
		"b": "\"c\"",
	}
	res := getHarvesterManifestContent(d)
	t.Log(res)
}

func TestGetHStatus(t *testing.T) {
	s := getHarvesterStatus()
	t.Log(s)
}

func TestGetFormattedServerURL(t *testing.T) {
	testCases := []struct {
		Name   string
		input  string
		output string
	}{
		{
			Name:   "ip",
			input:  "1.2.3.4",
			output: "https://1.2.3.4:6443",
		},
		{
			Name:   "domain name",
			input:  "example.org",
			output: "https://example.org:6443",
		},
		{
			Name:   "full",
			input:  "https://1.2.3.4:6443",
			output: "https://1.2.3.4:6443",
		},
	}
	for _, testCase := range testCases {
		got := getFormattedServerURL(testCase.input)
		assert.Equal(t, testCase.output, got)
	}
}

func TestGetSshKey(t *testing.T) {
	keys, err := getSSHKeysFromURL("https://github.com/gitlawr.keys")
	if err != nil {
		t.Error(err)
	}
	t.Log(keys)
}

func TestF(t *testing.T) {
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		// handle err
		for _, addr := range addrs {
			if v, ok := addr.(*net.IPNet); ok && !v.IP.IsLoopback() && v.IP.To4() != nil {
				t.Log(v.IP.String())
			}
		}
	}
}

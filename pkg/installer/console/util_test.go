package console

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/util"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

func TestGetSSHKeysFromURL(t *testing.T) {
	testCases := []struct {
		name         string
		httpResp     string
		pubKeysCount int
		expectError  string
	}{
		{
			name:         "Two public keys",
			httpResp:     string(util.LoadFixture(t, "keys")),
			pubKeysCount: 2,
		},
		{
			name:        "Invalid public key",
			httpResp:    "\nooxx",
			expectError: "fail to parse on line 2: ooxx",
		},
		{
			name:        "No public key",
			httpResp:    "",
			expectError: "no key found",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintln(w, testCase.httpResp) //nolint:errcheck
			}))
			defer ts.Close()

			pubKeys, err := getRemoteSSHKeys(ts.URL)
			if testCase.expectError != "" {
				assert.EqualError(t, err, testCase.expectError)
			} else {
				assert.Equal(t, nil, err)
				assert.Equal(t, testCase.pubKeysCount, len(pubKeys))
			}
		})
	}
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
		err    error
	}{
		{
			Name:   "ip",
			input:  "1.2.3.4",
			output: "https://1.2.3.4:443",
			err:    nil,
		},
		{
			Name:   "domain name",
			input:  "example.org",
			output: "https://example.org:443",
			err:    nil,
		},
		{
			Name:   "ip without port and scheme",
			input:  "1.1.1.1",
			output: "https://1.1.1.1:443",
			err:    nil,
		},
		{
			Name:   "domain without port and scheme",
			input:  "abc.org",
			output: "https://abc.org:443",
			err:    nil,
		},
		{
			Name:   "custom port",
			input:  "1.2.3.4:555",
			output: "",
			err:    errors.New("currently non-443 port are not allowed"),
		},
		{
			Name:   "ip with path",
			input:  "1.2.3.4/",
			output: "",
			err:    errors.New("path is not allowed in management address: /"),
		},
		{
			Name:   "domain with path",
			input:  "abc.org/test/abc",
			output: "",
			err:    errors.New("path is not allowed in management address: /test/abc"),
		},
	}
	for _, testCase := range testCases {
		got, err := getFormattedServerURL(testCase.input)
		assert.Equal(t, testCase.output, got)
		assert.Equal(t, testCase.err, err)
	}
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

func TestGetServerURLFromRancherdConfig(t *testing.T) {
	testCases := []struct {
		input []byte
		url   string
		err   error
	}{
		{
			input: []byte("role: cluster-init\nkubernetesVersion: v1.21.2+rke2r1"),
			url:   "",
			err:   nil,
		},
		{
			input: []byte("role: agent\nkubernetesVersion: v1.21.2+rke2r1\nserver: https://172.0.0.1:443"),
			url:   "https://172.0.0.1:443",
			err:   nil,
		},
	}

	for _, testCase := range testCases {
		url, err := getServerURLFromRancherdConfig(testCase.input)
		assert.Equal(t, testCase.url, url)
		assert.Equal(t, testCase.err, err)
	}
}

func TestValidateNTPServers(t *testing.T) {
	quit := make(chan interface{})
	mockNTPServers, err := startMockNTPServers(quit)
	if err != nil {
		t.Fatalf("can't start mock ntp servers, %v", err)
	}
	testCases := []struct {
		name        string
		input       []string
		expectError bool
	}{
		{
			name:        "Correct NTP Servers",
			input:       mockNTPServers,
			expectError: false,
		},
		{
			name:        "Empty input",
			input:       []string{},
			expectError: false,
		},
		{
			name:        "Invalid URL",
			input:       []string{"error"},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateNTPServers(testCase.input)
			if testCase.expectError {
				assert.NotNil(t, err)
			} else {
				if err != nil {
					t.Log(err)
				}
				assert.Nil(t, err)
			}
		})
	}
	close(quit)
}

func startMockNTPServers(quit chan interface{}) ([]string, error) {
	ntpServers := []string{}
	for i := 0; i < 2; i++ {
		listener, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ntpServers = append(ntpServers, listener.LocalAddr().String())

		go func(listener net.PacketConn) {
			defer listener.Close() //nolint:errcheck

			for {
				req := make([]byte, 48)
				_, addr, err := listener.ReadFrom(req)
				if err != nil {
					select {
					case <-quit:
						return
					default:
						continue
					}
				}
				go func(listener net.PacketConn, addr net.Addr) {
					listener.WriteTo(make([]byte, 48), addr) //nolint:errcheck,gosec
				}(listener, addr)
			}

		}(listener)
	}
	return ntpServers, nil
}

const (
	sampleSerialDiskOutput = `
{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "768.1M",
         "type": "loop",
         "wwn": null,
         "serial": null
      },{
         "name": "sda",
         "size": "250G",
         "type": "disk",
         "wwn": null,
         "serial": "serial-1",
         "children": [
            {
               "name": "0QEMU_QEMU_HARDDISK_serial-1",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null
            }
         ]
      },{
         "name": "sdb",
         "size": "250G",
         "type": "disk",
         "wwn": null,
         "serial": "serial-1",
         "children": [
            {
               "name": "0QEMU_QEMU_HARDDISK_serial-1",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null
            }
         ]
      },{
         "name": "sr0",
         "size": "5.8G",
         "type": "rom",
         "wwn": null,
         "serial": "QM00001"
      }
   ]
}
`

	reinstallDisks = `
{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "3G",
         "type": "loop",
         "wwn": null,
         "serial": null
      },{
         "name": "loop1",
         "size": "10G",
         "type": "loop",
         "wwn": null,
         "serial": null
      },{
         "name": "sda",
         "size": "10G",
         "type": "disk",
         "wwn": "0x60000000000000000e00000000010001",
         "serial": "beaf11",
         "children": [
            {
               "name": "sda1",
               "size": "2.5G",
               "type": "part",
               "wwn": "0x60000000000000000e00000000010001",
               "serial": null
            },{
               "name": "sda14",
               "size": "4M",
               "type": "part",
               "wwn": "0x60000000000000000e00000000010001",
               "serial": null
            },{
               "name": "sda15",
               "size": "106M",
               "type": "part",
               "wwn": "0x60000000000000000e00000000010001",
               "serial": null
            },{
               "name": "sda16",
               "size": "913M",
               "type": "part",
               "wwn": "0x60000000000000000e00000000010001",
               "serial": null
            }
         ]
      },{
         "name": "sr0",
         "size": "364K",
         "type": "rom",
         "wwn": null,
         "serial": "QM00001"
      },{
         "name": "vda",
         "size": "250G",
         "type": "disk",
         "wwn": null,
         "serial": null,
         "children": [
            {
               "name": "vda1",
               "size": "1M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "vda2",
               "size": "50M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "vda3",
               "size": "8G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "vda4",
               "size": "15G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "vda5",
               "size": "150G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "vda6",
               "size": "76.9G",
               "type": "part",
               "wwn": null,
               "serial": null
            }
         ]
      }
   ]
}
`

	preInstalledMultiPath = `
{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "768.4M",
         "type": "loop",
         "wwn": null,
         "serial": null
      },{
         "name": "sda",
         "size": "250G",
         "type": "disk",
         "wwn": null,
         "serial": "disk1",
         "children": [
            {
               "name": "sda1",
               "size": "1M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sda2",
               "size": "50M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sda3",
               "size": "8G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sda4",
               "size": "15G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sda5",
               "size": "150G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sda6",
               "size": "76.9G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "0QEMU_QEMU_HARDDISK_disk1",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null,
               "children": [
                  {
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part1",
                     "size": "1M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part2",
                     "size": "50M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part3",
                     "size": "8G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part4",
                     "size": "15G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part5",
                     "size": "150G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part6",
                     "size": "76.9G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  }
               ]
            }
         ]
      },{
         "name": "sdb",
         "size": "250G",
         "type": "disk",
         "wwn": null,
         "serial": "disk1",
         "children": [
            {
               "name": "sdb1",
               "size": "1M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sdb2",
               "size": "50M",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sdb3",
               "size": "8G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sdb4",
               "size": "15G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sdb5",
               "size": "150G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "sdb6",
               "size": "76.9G",
               "type": "part",
               "wwn": null,
               "serial": null
            },{
               "name": "0QEMU_QEMU_HARDDISK_disk1",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null,
               "children": [
                  {
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part1",
                     "size": "1M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part2",
                     "size": "50M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part3",
                     "size": "8G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part4",
                     "size": "15G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part5",
                     "size": "150G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "0QEMU_QEMU_HARDDISK_disk1-part6",
                     "size": "76.9G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  }
               ]
            }
         ]
      },{
         "name": "sr0",
         "size": "5.8G",
         "type": "rom",
         "wwn": null,
         "serial": "QM00001"
      }
   ]
}
`
	raidDisks = `{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "780.5M",
         "type": "loop",
         "wwn": null,
         "serial": null
      },{
         "name": "sda",
         "size": "447.1G",
         "type": "disk",
         "wwn": "0x600508b1001cec28a12a38168f7bb195",
         "serial": "PDNMF0ARH1614W",
         "children": [
            {
               "name": "3600508b1001cec28a12a38168f7bb195",
               "size": "447.1G",
               "type": "mpath",
               "wwn": null,
               "serial": null
            }
         ]
      },{
         "name": "sdb",
         "size": "447.1G",
         "type": "disk",
         "wwn": "0x600508b1001c3e956986e526698cd830",
         "serial": "PDNMF0ARH1614W",
         "children": [
            {
               "name": "sdb1",
               "size": "1M",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "sdb2",
               "size": "50M",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "sdb3",
               "size": "8G",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "sdb4",
               "size": "15G",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "sdb5",
               "size": "170G",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "sdb6",
               "size": "254G",
               "type": "part",
               "wwn": "0x600508b1001c3e956986e526698cd830",
               "serial": null
            },{
               "name": "3600508b1001c3e956986e526698cd830",
               "size": "447.1G",
               "type": "mpath",
               "wwn": null,
               "serial": null,
               "children": [
                  {
                     "name": "3600508b1001c3e956986e526698cd830-part1",
                     "size": "1M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "3600508b1001c3e956986e526698cd830-part2",
                     "size": "50M",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "3600508b1001c3e956986e526698cd830-part3",
                     "size": "8G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "3600508b1001c3e956986e526698cd830-part4",
                     "size": "15G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "3600508b1001c3e956986e526698cd830-part5",
                     "size": "170G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  },{
                     "name": "3600508b1001c3e956986e526698cd830-part6",
                     "size": "254G",
                     "type": "part",
                     "wwn": null,
                     "serial": null
                  }
               ]
            }
         ]
      },{
         "name": "sr0",
         "size": "1024M",
         "type": "rom",
         "wwn": null,
         "serial": "475652914613"
      }
   ]
}`

	existingHarvesterInstalls = `{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "4K",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop1",
         "size": "175.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop2",
         "size": "89.4M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop3",
         "size": "55.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop4",
         "size": "55.4M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop5",
         "size": "64M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop6",
         "size": "63.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop7",
         "size": "74.2M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop8",
         "size": "73.9M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop9",
         "size": "67.8M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop10",
         "size": "67.8M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop11",
         "size": "374.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop12",
         "size": "375.1M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop13",
         "size": "349.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop14",
         "size": "349.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop15",
         "size": "504.2M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop16",
         "size": "505.1M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop17",
         "size": "273.6M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop18",
         "size": "273M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop19",
         "size": "91.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop20",
         "size": "44.3M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop21",
         "size": "87M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop22",
         "size": "38.8M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop24",
         "size": "6.8M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop25",
         "size": "6.8M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "loop26",
         "size": "172.3M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "sda",
         "size": "838.1G",
         "type": "disk",
         "wwn": "0x600508b1001cc488149adefce15584da",
         "serial": "PDNLH0BRH7T0VY",
         "label": null,
         "children": [
            {
               "name": "sda1",
               "size": "1G",
               "type": "part",
               "wwn": "0x600508b1001cc488149adefce15584da",
               "serial": null,
               "label": null
            },{
               "name": "sda2",
               "size": "2G",
               "type": "part",
               "wwn": "0x600508b1001cc488149adefce15584da",
               "serial": null,
               "label": null
            },{
               "name": "sda3",
               "size": "835G",
               "type": "part",
               "wwn": "0x600508b1001cc488149adefce15584da",
               "serial": null,
               "label": null,
               "children": [
                  {
                     "name": "ubuntu--vg-ubuntu--lv",
                     "size": "835G",
                     "type": "lvm",
                     "wwn": null,
                     "serial": null,
                     "label": null
                  }
               ]
            }
         ]
      },{
         "name": "sdb",
         "size": "3.8G",
         "type": "disk",
         "wwn": null,
         "serial": "General_-0:0",
         "label": null,
         "children": [
            {
               "name": "sdb1",
               "size": "3.8G",
               "type": "part",
               "wwn": null,
               "serial": null,
               "label": null
            }
         ]
      },{
         "name": "sdc",
         "size": "250G",
         "type": "disk",
         "wwn": "0x60000000000000000e00000000010001",
         "serial": "1001",
         "label": null,
         "children": [
            {
               "name": "mpatha",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null,
               "label": null,
               "children": [
                  {
                     "name": "mpatha-part1",
                     "size": "64M",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_GRUB"
                  },{
                     "name": "mpatha-part2",
                     "size": "50M",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_OEM"
                  },{
                     "name": "mpatha-part3",
                     "size": "8G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_RECOVERY"
                  },{
                     "name": "mpatha-part4",
                     "size": "15G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_STATE"
                  },{
                     "name": "mpatha-part5",
                     "size": "150G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_PERSISTENT"
                  },{
                     "name": "mpatha-part6",
                     "size": "76.9G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "HARV_LH_DEFAULT"
                  }
               ]
            }
         ]
      },{
         "name": "sdd",
         "size": "250G",
         "type": "disk",
         "wwn": "0x60000000000000000e00000000010001",
         "serial": "1001",
         "label": null,
         "children": [
            {
               "name": "mpatha",
               "size": "250G",
               "type": "mpath",
               "wwn": null,
               "serial": null,
               "label": null,
               "children": [
                  {
                     "name": "mpatha-part1",
                     "size": "64M",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_GRUB"
                  },{
                     "name": "mpatha-part2",
                     "size": "50M",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_OEM"
                  },{
                     "name": "mpatha-part3",
                     "size": "8G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_RECOVERY"
                  },{
                     "name": "mpatha-part4",
                     "size": "15G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_STATE"
                  },{
                     "name": "mpatha-part5",
                     "size": "150G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "COS_PERSISTENT"
                  },{
                     "name": "mpatha-part6",
                     "size": "76.9G",
                     "type": "part",
                     "wwn": null,
                     "serial": null,
                     "label": "HARV_LH_DEFAULT"
                  }
               ]
            }
         ]
      },{
         "name": "nvme0n1",
         "size": "1.8T",
         "type": "disk",
         "wwn": "eui.0025385c2140432c",
         "serial": "S6S2NS0TC11162K",
         "label": null
      }
   ]
}`

	noValidDisks = `{
   "blockdevices": [
      {
         "name": "loop0",
         "size": "946.7M",
         "type": "loop",
         "wwn": null,
         "serial": null,
         "label": null
      },{
         "name": "sr0",
         "size": "1.4G",
         "type": "rom",
         "wwn": null,
         "serial": "QM00001",
         "label": null
      }
   ]
}`
)

func Test_getAllValidDiskOptions(t *testing.T) {
	defer func() { run = runCommand }()

	testCases := []struct {
		name                        string
		mockedRunCommandOutput      []byte
		expectedAllValidDiskOptions []widgets.Option
	}{
		{
			name: "Disks with serial number",
			mockedRunCommandOutput: []byte(sampleSerialDiskOutput),
			expectedAllValidDiskOptions: []widgets.Option{
				{
					Value: "/dev/sda",
					Text: "sda 250G",
				},
			},
		},
		{
			name: "Disks with existing data",
			mockedRunCommandOutput: []byte(reinstallDisks),
			expectedAllValidDiskOptions: []widgets.Option{
				{
					Value: "/dev/sda",
					Text: "sda 10G",
				},
				{
					Value: "/dev/vda",
					Text: "vda 250G",
				},
			},
		},
		{
			name: "Disks on existing installs",
			mockedRunCommandOutput: []byte(preInstalledMultiPath),
			expectedAllValidDiskOptions: []widgets.Option{
				{
					Value: "/dev/sda",
					Text: "sda 250G",
				},
			},
		},
		{
			name: "RAID disks",
			mockedRunCommandOutput: []byte(raidDisks),
			expectedAllValidDiskOptions: []widgets.Option{
				{
					Value: "/dev/sda",
					Text: "sda 447.1G",
				},
				{
					Value: "/dev/sdb",
					Text: "sdb 447.1G",
				},
			},
		},
		{
			name: "No Valid Disks",
			mockedRunCommandOutput: []byte(noValidDisks),
			expectedAllValidDiskOptions: []widgets.Option(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run = func(_ *exec.Cmd) ([]byte, error) {
				return []byte(tc.mockedRunCommandOutput), nil
			}

			assert := require.New(t)
			doc := NewDiskOptionsCache()
			err := doc.refresh()
			assert.NoError(err, "expected no error while fetching")
			actualAllValidDiskOptions := doc.getAllValidDiskOptions()
			assert.NoError(err, "expected no error while getting all valid disk options")
			assert.Equal(tc.expectedAllValidDiskOptions, actualAllValidDiskOptions)
		})
	}
}

func Test_getDataDisksOptions(t *testing.T) {
	defer func() { run = runCommand }()

	run = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(raidDisks), nil
	}

	assert := require.New(t)
	doc := NewDiskOptionsCache()
	err := doc.refresh()
	assert.NoError(err, "expected no error while fetching")

	hvstConfig := config.NewHarvesterConfig()

	// Set the first disk option as installation disk
	hvstConfig.Install.Device = doc.getAllValidDiskOptions()[0].Value
	assert.Equal(
		[]widgets.Option{
			widgets.Option{Value: "/dev/sda", Text: "Use the installation disk (sda 447.1G)"},
			widgets.Option{Value: "/dev/sdb", Text: "sdb 447.1G"},
		},
		doc.getDataDiskOptions(hvstConfig),
		)

	// Change the installation disk to the second disk option
	hvstConfig.Install.Device = doc.getAllValidDiskOptions()[1].Value
	assert.Equal(
		[]widgets.Option{
			widgets.Option{Value: "/dev/sdb", Text: "Use the installation disk (sdb 447.1G)"},
			widgets.Option{Value: "/dev/sda", Text: "sda 447.1G"},
		},
		doc.getDataDiskOptions(hvstConfig),
		)
}

func Test_getWipeDisksOptions(t *testing.T) {
	defer func() { run = runCommand }()

	run = func(_ *exec.Cmd) ([]byte, error) {
		return []byte(existingHarvesterInstalls), nil
	}

	assert := require.New(t)
	doc := NewDiskOptionsCache()
	err := doc.refresh()
	assert.NoError(err, "expected no error while fetching")

	hvstConfig := config.NewHarvesterConfig()
	assert.Equal(
		[]widgets.Option{
			widgets.Option{Value: "/dev/sdc", Text: "sdc 250G"},
		},
		doc.getWipeDisksOptions(hvstConfig),
		)

	hvstConfig.Install.Device = "/dev/sdc"
	hvstConfig.Install.DataDisk = ""
	assert.Equal([]widgets.Option(nil), doc.getWipeDisksOptions(hvstConfig), "expected to skip installation disk")

	hvstConfig.Install.Device = ""
	hvstConfig.Install.DataDisk = "/dev/sdc"
	assert.Equal([]widgets.Option(nil), doc.getWipeDisksOptions(hvstConfig), "expected to skip data disk")
}

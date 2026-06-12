package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/util"
)

func TestToEnv(t *testing.T) {
	obj := struct {
		PortNumber int    `json:"portNumber"`
		Enabled    bool   `json:"enabled"`
		Name       string `json:"name"`
	}{
		PortNumber: 8443,
		Enabled:    true,
		Name:       "harvester",
	}

	envs, err := ToEnv("PREFIX_", &obj)
	if err != nil {
		t.Fatalf("ToEnv() error = %v", err)
	}

	want := map[string]struct{}{
		"PREFIX_PORT_NUMBER=8443": {},
		"PREFIX_ENABLED=true":     {},
		"PREFIX_NAME=harvester":   {},
	}

	got := make(map[string]struct{})
	for _, env := range envs {
		got[env] = struct{}{}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
}

func TestReadUserData(t *testing.T) {
	config, err := readUserData("./testdata/userdata.yaml")
	assert.NoError(t, err, "expected no error during loading of userdata")
	assert.Equal(t, config.Token, "token", "expected token to be token")
	assert.Equal(t, config.OS.Password, "p@ssword", "expected password to be p@ssword")
	assert.Equal(t, config.Install.ManagementInterface.Method, "dhcp", "expected network mode to be dhcp")
}

func TestReadConfigFromMap1(t *testing.T) {
	data, err := util.ParseCmdLine(
		"harvester.scheme_version=2 "+
			"harvester.install.skipchecks=true "+
			"harvester.install.silent=yes "+
			"harvester.install.debug=false "+
			"harvester.install.forceGpt=no "+
			"harvester.install.management_interface.method=dhcp "+
			"harvester.install.management_interface.bond_options.mode=balance-tlb "+
			"harvester.install.management_interface.bond_options.miimon=100 "+
			"harvester.install.management_interface.mtu=1500 "+
			"harvester.install.management_interface.vlan_id=1 "+
			"harvester.install.wipeDisksList=/dev/sda "+
			"harvester.install.wipeDisksList=/dev/sdb",
		kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Equal(t, config.SchemeVersion, uint32(2), "expected scheme version to be 2")
	assert.True(t, config.Install.SkipChecks, "expected skip checks to be true")
	assert.True(t, config.Install.Silent, "expected silent to be true")
	assert.False(t, config.Install.Debug, "expected debug to be false")
	assert.False(t, config.Install.ForceGPT, "expected forceGpt to be false")
	assert.Equal(t, config.Install.ManagementInterface.Method, "dhcp", "expected network mode to be dhcp")
	assert.Len(t, config.Install.ManagementInterface.BondOptions, 2, "expected 2 bond options")
	assert.Equal(t, config.Install.ManagementInterface.BondOptions["mode"], "balance-tlb", "expected bond mode to be balance-tlb")
	assert.Equal(t, config.Install.ManagementInterface.BondOptions["miimon"], "100", "expected bond miimon to be 100")
	assert.Equal(t, config.Install.ManagementInterface.MTU, 1500, "expected MTU to be 1500")
	assert.Equal(t, config.Install.ManagementInterface.VlanID, 1, "expected VLAN ID to be 1")
	assert.Len(t, config.Install.WipeDisksList, 2, "expected 2 entries in wipe disks list")
	assert.Equal(t, config.Install.WipeDisksList[0], "/dev/sda")
	assert.Equal(t, config.Install.WipeDisksList[1], "/dev/sdb")
}

func TestReadConfigFromMap2(t *testing.T) {
	data, err := util.ParseCmdLine("harvester.scheme_version=1 harvester.install.management_interface.mtu=aaa", kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.Error(t, err, "expected an error when processing the config data")
	assert.Equal(t, uint32(0), config.SchemeVersion, "expected scheme version to be 0")
	assert.Equal(t, config.Install.ManagementInterface.MTU, 0, "expected MTU to be 0")
}

func TestReadConfigFromMap3(t *testing.T) {
	data, err := util.ParseCmdLine("harvester.scheme_version=1 harvester.install.debug=bbb", kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Equal(t, uint32(1), config.SchemeVersion, "expected scheme version to be 1")
	assert.Equal(t, config.Install.Debug, false, "expected debug to be false")
}

func TestReadConfigFromMap4(t *testing.T) {
	data, err := util.ParseCmdLine("harvester.scheme_version=2 harvester.install.management_interface.vlan_id=1.5", kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.Error(t, err, "expected an error when processing the config data")
	assert.Equal(t, uint32(0), config.SchemeVersion, "expected scheme version to be 0")
	assert.Equal(t, config.Install.ManagementInterface.VlanID, 0, "expected VLAN ID to be 0")
}

// TestReadConfigFromMap5 test several entries in a string list.
func TestReadConfigFromMap5(t *testing.T) {
	data, err := util.ParseCmdLine("harvester.scheme_version=2 harvester.install.wipeDisksList=111 harvester.install.wipeDisksList=aaa", kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Equal(t, uint32(2), config.SchemeVersion, "expected scheme version to be 2")
	assert.Len(t, config.Install.WipeDisksList, 2, "expected 2 entries in wipe disks list")
	assert.Equal(t, config.Install.WipeDisksList[0], "111")
	assert.Equal(t, config.Install.WipeDisksList[1], "aaa")
}

// TestReadConfigFromMap6 test a single entry in a string list. This will test
// the `NewToSlice` converter function.
func TestReadConfigFromMap6(t *testing.T) {
	data, err := util.ParseCmdLine("harvester.install.wipeDisksList=bbb", kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Len(t, config.Install.WipeDisksList, 1, "expected 1 entries in wipe disks list")
	assert.Equal(t, config.Install.WipeDisksList[0], "bbb")
}

func TestReadConfigFromMap7(t *testing.T) {
	data, err := util.ParseCmdLine(
		`harvester.install.management_interface.interfaces="hwAddr:be:44:8c:b0:5d:f2,name:ens1" `+
			`harvester.install.management_interface.interfaces="hwAddr:af:6a:ad:0d:06:d3,ens2" `+
			`harvester.install.management_interface.interfaces="name:ens3,6a:fe:da:c4:37:a4" `+
			`harvester.install.management_interface.interfaces="ens4,10:fe:71:05:57:fd"`,
		kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Len(t, config.Install.ManagementInterface.Interfaces, 4, "expected 4 interface entries")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[0].Name, "ens1")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[0].HwAddr, "be:44:8c:b0:5d:f2")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[1].Name, "ens2")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[1].HwAddr, "af:6a:ad:0d:06:d3")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[2].Name, "ens3")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[2].HwAddr, "6a:fe:da:c4:37:a4")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[3].Name, "ens4")
	assert.Equal(t, config.Install.ManagementInterface.Interfaces[3].HwAddr, "10:fe:71:05:57:fd")
}

func Test_parseCmdLineWithNilValue(t *testing.T) {
	data, err := util.ParseCmdLine(
		`harvester.install.harvester.longhorn.defaultSettings.guaranteedEngineManagerCPU=nil harvester.install.harvester.longhorn.defaultSettings.guaranteedReplicaManagerCPU=2`,
		kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	assert.Equal(t, map[string]any{
		"install": map[string]any{
			"harvester": map[string]any{
				"longhorn": map[string]any{
					"defaultSettings": map[string]any{
						"guaranteedEngineManagerCPU":  "nil",
						"guaranteedReplicaManagerCPU": "2",
					},
				},
			},
		}}, data)
	config, err := readConfigFromMap(data)
	assert.NoError(t, err, "expected no error when processing the config data")
	assert.Nil(t, config.Install.Harvester.Longhorn.DefaultSettings.GuaranteedEngineManagerCPU)
	assert.NotNil(t, config.Install.Harvester.Longhorn.DefaultSettings.GuaranteedReplicaManagerCPU)
	assert.Equal(t, *config.Install.Harvester.Longhorn.DefaultSettings.GuaranteedReplicaManagerCPU, uint32(2))
}

func Test_parseCmdLineWithError(t *testing.T) {
	data, err := util.ParseCmdLine(
		`harvester.install.harvester.longhorn.defaultSettings.guaranteedEngineManagerCPU=abc`,
		kernelParamPrefix)
	assert.NoError(t, err, "expected no error when parsing the command line")
	_, err = readConfigFromMap(data)
	assert.Error(t, err, "expected error when processing the config data")
	assert.ErrorContains(t, err, "failed to convert field \"guaranteedEngineManagerCPU\":")
}

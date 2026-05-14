package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	yipSchema "github.com/rancher/yip/pkg/schema"
	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/util"
)

func TestMain(m *testing.M) {
	//config.NMConnectionPath, err := os.MkdirTemp("/tmp", "cos-test-")
	dir, err := os.MkdirTemp("", "cos-test-")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	defer os.RemoveAll(dir)

	// So UpdateManagementInterfaceConfig will work
	NMConnectionPath = dir

	m.Run()
}

func TestCalcCosPersistentPartSize(t *testing.T) {
	testCases := []struct {
		diskSize      uint64
		partitionSize string
		result        uint64
		err           string
	}{
		{
			diskSize:      300,
			partitionSize: "150Gi",
			result:        153600,
		},
		{
			diskSize:      500,
			partitionSize: "153600Mi",
			result:        153600,
		},
		{
			diskSize:      250,
			partitionSize: "240Gi",
			err:           "partition size is too large. Maximum 176Gi is allowed",
		},
		{
			diskSize:      150,
			partitionSize: "100Gi",
			err:           "installation disk size is too small. Minimum 250Gi is required",
		},
		{
			diskSize:      300,
			partitionSize: "153600Ki",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      2000,
			partitionSize: "1.5Ti",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
		{
			diskSize:      500,
			partitionSize: "abcd",
			err:           "partition size must end with 'Mi' or 'Gi'. Decimals and negatives are not allowed",
		},
	}

	for _, tc := range testCases {
		result, err := calcCosPersistentPartSize(tc.diskSize, tc.partitionSize, false)
		assert.Equal(t, tc.result, result)
		if err != nil {
			assert.EqualError(t, err, tc.err)
		}
	}
}

func TestConvertToCos_SSHKeysInYipNetworkStage(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)

	yipConfig, err := ConvertToCOS(conf)
	assert.NoError(t, err)

	assert.Equal(t, yipConfig.Stages["network"][0].SSHKeys["rancher"], conf.OS.SSHAuthorizedKeys)
	assert.Nil(t, yipConfig.Stages["initramfs"][0].SSHKeys)
}

func TestConvertToCos_InstallModeOnly(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)
	conf.Mode = ModeInstall
	yipConfig, err := ConvertToCOS(conf)
	assert.NoError(t, err)

	assert.NotNil(t, yipConfig.Stages["rootfs"])
	assert.Len(t, yipConfig.Stages["network"][0].SSHKeys, 0)
	assert.NotNil(t, yipConfig.Stages["initramfs"])
	assert.Equal(t, yipConfig.Stages["initramfs"][0].Users[cosLoginUser], yipSchema.User{
		PasswordHash: conf.OS.Password,
	})
}

func Test_GenerateRancherdConfig(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)
	conf.Mode = ModeInstall
	yipConfig, err := GenerateRancherdConfig(conf)
	assert.NoError(t, err)
	assert.Equal(t, yipConfig.Stages["live"][0].TimeSyncd["NTP"], strings.Join(conf.OS.NTPServers, " "))
}

func TestGenBootstrapResources(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)
	bootstrapResources, err := genBootstrapResources(conf)
	assert.NoError(t, err)
	assert.True(t, len(bootstrapResources) > 0)
}

func TestConvertToCos_VerifyNetworkCreateMode(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)
	yipConfig, err := ConvertToCOS(conf)
	assert.NoError(t, err)
	assert.True(t, containsFile(yipConfig.Stages["initramfs"][0].Files, "/etc/rancher/rancherd/config.yaml"))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-mgmt.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bridge-mgmt.nmconnection")))
	assert.False(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "vlan-mgmt.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens0.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens3.nmconnection")))

}

func TestConvertToCos_VerifyNetworkCreateModeVlan(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	conf.ManagementInterface.VlanID = 2
	assert.NoError(t, err)
	yipConfig, err := ConvertToCOS(conf)
	assert.NoError(t, err)
	assert.True(t, containsFile(yipConfig.Stages["initramfs"][0].Files, "/etc/rancher/rancherd/config.yaml"))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-mgmt.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bridge-mgmt.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "vlan-mgmt.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens0.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens3.nmconnection")))

}

func TestConvertToCos_VerifyNetworkInstallMode(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)
	conf.Mode = ModeInstall
	_, err = ConvertToCOS(conf)
	assert.NoError(t, err)
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens0.nmconnection")))
	assert.True(t, fileExists(fmt.Sprintf("%s/%s", NMConnectionPath, "bond-slave-ens3.nmconnection")))
}

func TestConvertToCos_Remove_CPUManagerState(t *testing.T) {
	conf, err := LoadHarvesterConfig(util.LoadFixture(t, "harvester-config.yaml"))
	assert.NoError(t, err)

	yipConfig, err := ConvertToCOS(conf)
	assert.NoError(t, err)

	assert.Contains(t, yipConfig.Stages["initramfs"][0].Commands, "rm -f /var/lib/kubelet/cpu_manager_state")
}

func TestOverwriteSSHDComponent_DisablePasswordAuth(t *testing.T) {
	conf := NewHarvesterConfig()
	conf.OS.SSHD.DisablePasswordAuth = true

	overwriteSSHDComponent(conf)

	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("mkdir -p %s", SSHConfigFolder))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'PasswordAuthentication no' > %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'KbdInteractiveAuthentication no' >> %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'UsePAM no' >> %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
}

func TestOverwriteSSHDComponent_DisablePasswordAuth_NotSet(t *testing.T) {
	conf := NewHarvesterConfig()
	conf.OS.SSHD.DisablePasswordAuth = false

	overwriteSSHDComponent(conf)

	for _, cmd := range conf.OS.AfterInstallChrootCommands {
		assert.NotContains(t, cmd, SSHPasswordConfigFile)
	}
}

func TestOverwriteSSHDComponent_SFTP(t *testing.T) {
	conf := NewHarvesterConfig()
	conf.OS.SSHD.SFTP = true

	overwriteSSHDComponent(conf)

	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("mkdir -p %s", SSHConfigFolder))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'Subsystem\tsftp\t/usr/libexec/ssh/sftp-server' > %s/sftp.conf", SSHConfigFolder))
}

func TestOverwriteSSHDComponent_BothEnabled(t *testing.T) {
	conf := NewHarvesterConfig()
	conf.OS.SSHD.SFTP = true
	conf.OS.SSHD.DisablePasswordAuth = true

	overwriteSSHDComponent(conf)

	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("mkdir -p %s", SSHConfigFolder))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'PasswordAuthentication no' > %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'KbdInteractiveAuthentication no' >> %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
	assert.Contains(t, conf.OS.AfterInstallChrootCommands, fmt.Sprintf("echo 'UsePAM no' >> %s/%s", SSHConfigFolder, SSHPasswordConfigFile))
}

func fileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return err == nil
}

func containsFile(files []yipSchema.File, fileName string) bool {
	for _, v := range files {
		if v.Path == fileName {
			return true
		}
	}
	return false
}

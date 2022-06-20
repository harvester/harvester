package api_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"

	"github.com/harvester/harvester/pkg/builder"
)

const (
	testCreator        = "harvester-integration-test"
	testVMGenerateName = "test-"
	testVMNamespace    = "default"

	testVMCPUCores       = 1
	testVMMemory         = "256Mi"
	testVMUpdatedCPUCore = 2
	testVMUpdatedMemory  = "200Mi"

	testVMDiskSize = "10Mi"

	testVMInterfaceName  = "default"
	testVMInterfaceModel = "virtio"

	testVMBlankDiskName                = "blankdisk"
	testVMCDRomDiskName                = "cdromdisk"
	testVMContainerDiskName            = "containerdisk"
	testVMContainerDiskImageName       = "kubevirt/fedora-cloud-container-disk-demo:v0.35.0"
	testVMRemoveDiskName               = "sparedisk"
	testVMContainerDiskImagePullPolicy = builder.DefaultImagePullPolicy
	testVMCloudInitDiskName            = builder.CloudInitDiskName

	testVMDefaultDiskBus = builder.DiskBusVirtio
	testVMCDRomBus       = builder.DiskBusSata

	testVMCloudInitUserDataTemplate = `
#cloud-config
user: %s
password: %s
chpasswd: { expire: False }
ssh_pwauth: True`

	testVMCloudInitNetworkDataTemplate = `
network:
  version: 1
  config:
  - type: physical
    name: eth0
    subnets:
    - type: static
      address: %s
      gateway: %s`

	testVMExpect = `
#!/usr/bin/env expect

set vmi [lindex $argv 0 ]
set username [lindex $argv 1 ]
set password [lindex $argv 2 ]
set ip [lindex $argv 3 ]


spawn virtctl console $vmi

set timeout 10
expect {
  "Successfully connected" {send "\r"}
  timeout {exit 1}
}

set timeout 300
expect {
  "login:" {send "$username\r"}
  timeout {exit 1}
}

set timeout 10
expect {
  "Password:" {send "$password\n"}
  timeout {exit 1}
}

set timeout 5
expect {
  "Login incorrect"   {exit 2}
  timeout {send "ip a | grep $ip\n"}
}

expect {
  "$ip" {send "exit\n"; expect eof; exit 0}
  timeout {send "exit\n"; expect eof; exit 1}
}`
)

type VMCloudInit struct {
	Name     string
	UserName string
	Password string
	Address  string
	Gateway  string
}

func (c *VMCloudInit) Check() error {
	fileName, err := CreateTmpFile(os.TempDir(), "tmp-expect-", testVMExpect, 0777)
	if err != nil {
		return err
	}
	args := []string{c.Name, c.UserName, c.Password, c.Address}
	command := fmt.Sprintf("expect -f %s %s", fileName, strings.Join(args, " "))
	return Exec(command)
}

func Exec(command string) error {
	cmd := exec.Command("bash", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		ginkgo.GinkgoT().Errorf("failed to exec command %s, out: %s, err: %v", cmd, out, err)
		return err
	}
	ginkgo.GinkgoT().Logf("exec command %s, out: %s", cmd, out)
	return nil
}

func CreateTmpFile(dir, pattern, content string, mode os.FileMode) (string, error) {
	tmpFile, err := ioutil.TempFile(dir, pattern)
	if err != nil {
		return "", err
	}
	if _, err = tmpFile.WriteString(content); err != nil {
		return "", err
	}
	fileName := tmpFile.Name()
	if err = os.Chmod(fileName, mode); err != nil {
		return "", err
	}
	return fileName, err
}

func NewDefaultTestVMBuilder(labels map[string]string) *builder.VMBuilder {
	return builder.NewVMBuilder(testCreator).Namespace(testVMNamespace).Labels(labels).
		CPU(testVMCPUCores).Memory(testVMMemory).Run(false)
}

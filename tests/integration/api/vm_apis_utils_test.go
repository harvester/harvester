package api_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	cdiv1alpha1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"

	"github.com/rancher/harvester/pkg/util"
	"github.com/rancher/harvester/tests/framework/fuzz"
)

const (
	testVMGenerateName = "test-"
	testVMNamespace    = "default"

	testVMCPUCores = 1
	testVMMemory   = "256Mi"

	testVMManagementNetworkName   = "default"
	testVMManagementInterfaceName = "default"
	testVMInterfaceModel          = "virtio"

	testVMCDRomDiskName                = "cdromdisk"
	testVMCloudInitDiskName            = "cloudinitdisk"
	testVMContainerDiskName            = "containerdisk"
	testVMContainerDiskImageName       = "kubevirt/fedora-cloud-container-disk-demo:v0.35.0"
	testVMContainerDiskImagePullPolicy = corev1.PullIfNotPresent

	testVMDefaultDiskBus = "virtio"
	testVMCDRomBus       = "sata"

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

type VMBuilder struct {
	vm *kubevirtv1.VirtualMachine
}

func NewVMBuilder(vm *kubevirtv1.VirtualMachine) *VMBuilder {
	return &VMBuilder{
		vm: vm,
	}
}

func NewDefaultTestVMBuilder() *VMBuilder {
	objectMeta := metav1.ObjectMeta{
		Namespace:    testVMNamespace,
		GenerateName: testVMGenerateName,
		Labels:       testResourceLabels,
	}
	running := pointer.BoolPtr(false)
	cpu := &kubevirtv1.CPU{
		Cores: testVMCPUCores,
	}
	resources := kubevirtv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(testVMMemory),
		},
	}
	interfaces := []kubevirtv1.Interface{
		{
			Name:  testVMManagementInterfaceName,
			Model: testVMInterfaceModel,
			InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
				Masquerade: &kubevirtv1.InterfaceMasquerade{},
			},
		},
	}
	networks := []kubevirtv1.Network{
		{
			Name: testVMManagementNetworkName,
			NetworkSource: kubevirtv1.NetworkSource{
				Pod: &kubevirtv1.PodNetwork{},
			},
		},
	}
	template := &kubevirtv1.VirtualMachineInstanceTemplateSpec{
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				CPU: cpu,
				Devices: kubevirtv1.Devices{
					Disks:      []kubevirtv1.Disk{},
					Interfaces: interfaces,
				},
				Resources: resources,
			},
			Networks: networks,
			Volumes:  []kubevirtv1.Volume{},
		},
	}
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: objectMeta,
		Spec: kubevirtv1.VirtualMachineSpec{
			Running:             running,
			Template:            template,
			DataVolumeTemplates: []kubevirtv1.DataVolumeTemplateSpec{},
		},
	}
	return &VMBuilder{
		vm: vm,
	}
}

func (v *VMBuilder) Name(name string) *VMBuilder {
	v.vm.ObjectMeta.Name = name
	v.vm.ObjectMeta.GenerateName = ""
	return v
}

func (v *VMBuilder) Namespace(namespace string) *VMBuilder {
	v.vm.ObjectMeta.Namespace = namespace
	return v
}

func (v *VMBuilder) Memory(memory string) *VMBuilder {
	v.vm.Spec.Template.Spec.Domain.Resources.Requests = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse(memory),
	}
	return v
}

func (v *VMBuilder) CPU(cores uint32) *VMBuilder {
	v.vm.Spec.Template.Spec.Domain.CPU.Cores = cores
	return v
}

func (v *VMBuilder) Blank() *VMBuilder {
	v.DataVolume("disk-blank", "10Mi")
	return v
}

func (v *VMBuilder) Image(imageName string) *VMBuilder {
	sourceHTTPURL := fmt.Sprintf("%s/%s/%s", options.ImageStorageEndpoint, util.BucketName, imageName)
	return v.DataVolume("disk-image", "10Gi", sourceHTTPURL)
}

func (v *VMBuilder) DataVolume(diskName, storageSize string, sourceHTTPURL ...string) *VMBuilder {
	volumeMode := corev1.PersistentVolumeFilesystem
	dataVolumeName := fmt.Sprintf("%s-%s-%s", v.vm.Name, diskName, fuzz.String(5))
	// DataVolumeTemplates
	dataVolumeTemplates := v.vm.Spec.DataVolumeTemplates
	dataVolumeSpecSource := cdiv1alpha1.DataVolumeSource{
		Blank: &cdiv1alpha1.DataVolumeBlankImage{},
	}

	if len(sourceHTTPURL) > 0 && sourceHTTPURL[0] != "" {
		dataVolumeSpecSource = cdiv1alpha1.DataVolumeSource{
			HTTP: &cdiv1alpha1.DataVolumeSourceHTTP{
				URL: sourceHTTPURL[0],
			},
		}
	}
	dataVolumeTemplate := kubevirtv1.DataVolumeTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        dataVolumeName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: cdiv1alpha1.DataVolumeSpec{
			Source: dataVolumeSpecSource,
			PVC: &corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
				VolumeMode: &volumeMode,
			},
		},
	}
	dataVolumeTemplates = append(dataVolumeTemplates, dataVolumeTemplate)
	v.vm.Spec.DataVolumeTemplates = dataVolumeTemplates
	// Disks
	disks := v.vm.Spec.Template.Spec.Domain.Devices.Disks
	disks = append(disks, kubevirtv1.Disk{
		Name: diskName,
		DiskDevice: kubevirtv1.DiskDevice{
			Disk: &kubevirtv1.DiskTarget{
				Bus: testVMDefaultDiskBus,
			},
		},
	})
	v.vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	// Volumes
	volumes := v.vm.Spec.Template.Spec.Volumes
	volumes = append(volumes, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			DataVolume: &kubevirtv1.DataVolumeSource{
				Name: dataVolumeName,
			},
		},
	})
	v.vm.Spec.Template.Spec.Volumes = volumes
	return v
}

func (v *VMBuilder) ExistingDataVolume(diskName, dataVolumeName string) *VMBuilder {
	// Disks
	disks := v.vm.Spec.Template.Spec.Domain.Devices.Disks
	disks = append(disks, kubevirtv1.Disk{
		Name: diskName,
		DiskDevice: kubevirtv1.DiskDevice{
			Disk: &kubevirtv1.DiskTarget{
				Bus: testVMDefaultDiskBus,
			},
		},
	})
	v.vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	// Volumes
	volumes := v.vm.Spec.Template.Spec.Volumes
	volumes = append(volumes, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			DataVolume: &kubevirtv1.DataVolumeSource{
				Name: dataVolumeName,
			},
		},
	})
	v.vm.Spec.Template.Spec.Volumes = volumes
	return v
}

func (v *VMBuilder) ContainerDisk(diskName, imageName string, isCDRom bool) *VMBuilder {
	// Disks
	disks := v.vm.Spec.Template.Spec.Domain.Devices.Disks
	diskDevice := kubevirtv1.DiskDevice{
		Disk: &kubevirtv1.DiskTarget{
			Bus: testVMDefaultDiskBus,
		},
	}
	if isCDRom {
		diskDevice = kubevirtv1.DiskDevice{
			CDRom: &kubevirtv1.CDRomTarget{
				Bus: testVMCDRomBus,
			},
		}
	}
	disks = append(disks, kubevirtv1.Disk{
		Name:       diskName,
		DiskDevice: diskDevice,
	})
	v.vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	// Volumes
	volumes := v.vm.Spec.Template.Spec.Volumes
	volumes = append(volumes, kubevirtv1.Volume{
		Name: diskName,
		VolumeSource: kubevirtv1.VolumeSource{
			ContainerDisk: &kubevirtv1.ContainerDiskSource{
				Image:           imageName,
				ImagePullPolicy: testVMContainerDiskImagePullPolicy,
			},
		},
	})
	v.vm.Spec.Template.Spec.Volumes = volumes
	return v
}

func (v *VMBuilder) Container(containerImageName ...string) *VMBuilder {
	imageName := testVMContainerDiskImageName
	if len(containerImageName) > 0 {
		imageName = containerImageName[0]
	}
	return v.ContainerDisk(testVMContainerDiskName, imageName, false)
}

func (v *VMBuilder) CDRom(containerImageName ...string) *VMBuilder {
	imageName := testVMContainerDiskImageName
	if len(containerImageName) > 0 {
		imageName = containerImageName[0]
	}
	return v.ContainerDisk(testVMCDRomDiskName, imageName, true)
}

func (v *VMBuilder) CloudInit(vmCloudInit *VMCloudInit) *VMBuilder {
	// Disks
	disks := v.vm.Spec.Template.Spec.Domain.Devices.Disks
	for _, disk := range disks {
		if disk.Name == testVMCloudInitDiskName {
			return v
		}
	}

	disks = append(disks, kubevirtv1.Disk{
		Name: testVMCloudInitDiskName,
		DiskDevice: kubevirtv1.DiskDevice{
			Disk: &kubevirtv1.DiskTarget{
				Bus: testVMDefaultDiskBus,
			},
		},
	})
	v.vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	// Volumes
	var userData, networkData string
	if vmCloudInit != nil {
		if vmCloudInit.Password != "" {
			userData = fmt.Sprintf(testVMCloudInitUserDataTemplate, vmCloudInit.UserName, vmCloudInit.Password)
		}
		if vmCloudInit.Address != "" && vmCloudInit.Gateway != "" {
			networkData = fmt.Sprintf(testVMCloudInitNetworkDataTemplate, vmCloudInit.Address, vmCloudInit.Gateway)
		}
	}
	volumes := v.vm.Spec.Template.Spec.Volumes
	volumes = append(volumes, kubevirtv1.Volume{
		Name: testVMCloudInitDiskName,
		VolumeSource: kubevirtv1.VolumeSource{
			CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
				UserData:    userData,
				NetworkData: networkData,
			},
		},
	})
	v.vm.Spec.Template.Spec.Volumes = volumes
	return v
}

func (v *VMBuilder) Network(networkName string) *VMBuilder {
	// Networks
	networks := v.vm.Spec.Template.Spec.Networks
	networks = append(networks, kubevirtv1.Network{
		Name: networkName,
		NetworkSource: kubevirtv1.NetworkSource{
			Multus: &kubevirtv1.MultusNetwork{
				NetworkName: networkName,
				Default:     false,
			},
		},
	})
	v.vm.Spec.Template.Spec.Networks = networks
	// Interfaces
	interfaces := v.vm.Spec.Template.Spec.Domain.Devices.Interfaces
	interfaces = append(interfaces, kubevirtv1.Interface{
		Name:  networkName,
		Model: testVMInterfaceModel,
		InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
			Bridge: &kubevirtv1.InterfaceBridge{},
		},
	})
	v.vm.Spec.Template.Spec.Domain.Devices.Interfaces = interfaces
	return v
}

func (v *VMBuilder) Run() *kubevirtv1.VirtualMachine {
	v.vm.Spec.Running = pointer.BoolPtr(true)
	return v.VM()
}

func (v *VMBuilder) VM() *kubevirtv1.VirtualMachine {
	return v.vm
}

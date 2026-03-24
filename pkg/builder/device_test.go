package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestHostDevice(t *testing.T) {
	builder := NewVMBuilder("test")

	// First host device
	builder.HostDevice("qat", "intel.com/qat", "")
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, 1)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, kubevirtv1.HostDevice{
		Name:       "qat",
		DeviceName: "intel.com/qat",
		Tag:        "",
	})

	// Can force-add same host device again
	builder.HostDevice("qat", "intel.com/qat", "")
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, 2)
}

func TestAddHostDevice(t *testing.T) {
	builder := NewVMBuilder("test")

	// Add first Host device
	builder.AddHostDevice("qat1", "intel.com/qat", "")
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, 1)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, kubevirtv1.HostDevice{
		Name:       "qat1",
		DeviceName: "intel.com/qat",
		Tag:        "",
	})

	// Can not add same device again
	builder.AddHostDevice("qat1", "intel.com/qat", "")
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, 1)

	// Can add different device
	builder.AddHostDevice("qat2", "intel.com/qat", "")
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, 2)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, kubevirtv1.HostDevice{
		Name:       "qat1",
		DeviceName: "intel.com/qat",
		Tag:        "",
	})
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices, kubevirtv1.HostDevice{
		Name:       "qat2",
		DeviceName: "intel.com/qat",
		Tag:        "",
	})
}

func TestGPU(t *testing.T) {
	builder := NewVMBuilder("test")

	// First GPU
	builder.GPU("gpu", "nvidia.com/TU104GL_Tesla_T4", "", &kubevirtv1.VGPUOptions{})
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, 1)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, kubevirtv1.GPU{
		Name:              "gpu",
		DeviceName:        "nvidia.com/TU104GL_Tesla_T4",
		Tag:               "",
		VirtualGPUOptions: &kubevirtv1.VGPUOptions{},
	})

	// Can force-add same GPU again
	builder.GPU("gpu", "nvidia.com/TU104GL_Tesla_T4", "", &kubevirtv1.VGPUOptions{})
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, 2)
}

func TestAddGPU(t *testing.T) {
	builder := NewVMBuilder("test")

	// Add first GPU
	builder.AddGPU("gpu1", "nvidia.com/TU104GL_Tesla_T4", "", &kubevirtv1.VGPUOptions{})
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, 1)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, kubevirtv1.GPU{
		Name:              "gpu1",
		DeviceName:        "nvidia.com/TU104GL_Tesla_T4",
		Tag:               "",
		VirtualGPUOptions: &kubevirtv1.VGPUOptions{},
	})

	// Can not add the same GPU again
	builder.AddGPU("gpu1", "nvidia.com/TU104GL_Tesla_T4", "", &kubevirtv1.VGPUOptions{})
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, 1)

	// Can add different GPU
	builder.AddGPU("gpu2", "nvidia.com/TU104GL_Tesla_T4", "", &kubevirtv1.VGPUOptions{})
	assert.NotEmpty(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs)
	assert.Len(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, 2)
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, kubevirtv1.GPU{
		Name:              "gpu1",
		DeviceName:        "nvidia.com/TU104GL_Tesla_T4",
		Tag:               "",
		VirtualGPUOptions: &kubevirtv1.VGPUOptions{},
	})
	assert.Contains(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs, kubevirtv1.GPU{
		Name:              "gpu2",
		DeviceName:        "nvidia.com/TU104GL_Tesla_T4",
		Tag:               "",
		VirtualGPUOptions: &kubevirtv1.VGPUOptions{},
	})
}

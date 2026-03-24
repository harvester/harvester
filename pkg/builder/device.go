package builder

import (
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	InputTypeTablet = "tablet"
	InputBusUSB     = "usb"
	InputBusVirtio  = "virtio"
)

func (v *VMBuilder) Input(inputName string, inputType kubevirtv1.InputType, inputBus kubevirtv1.InputBus) *VMBuilder {
	inputs := v.VirtualMachine.Spec.Template.Spec.Domain.Devices.Inputs
	input := kubevirtv1.Input{
		Name: inputName,
		Type: inputType,
	}
	if inputBus != "" {
		input.Bus = inputBus
	}
	inputs = append(inputs, input)
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.Inputs = inputs
	return v
}

// Unconditionally adds the specified host device
func (v *VMBuilder) HostDevice(name, hostDeviceName, tag string) *VMBuilder {
	hostDevices := v.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices
	hostDevice := kubevirtv1.HostDevice{
		Name:       name,
		DeviceName: hostDeviceName,
	}
	if tag != "" {
		hostDevice.Tag = tag
	}
	hostDevices = append(hostDevices, hostDevice)
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices = hostDevices
	return v
}

// Adds the specified host device if it does not yet exist
func (v *VMBuilder) AddHostDevice(name, hostDeviceName, tag string) *VMBuilder {
	hostDevices := v.VirtualMachine.Spec.Template.Spec.Domain.Devices.HostDevices

	found := false
	for _, hdev := range hostDevices {
		if hdev.Name == name && hdev.DeviceName == hostDeviceName {
			found = true
		}
	}

	if !found {
		v.HostDevice(name, hostDeviceName, tag)
	}
	return v
}

// Unconditionally adds the speficied GPU device
func (v *VMBuilder) GPU(name, hostDeviceName, tag string, opts *kubevirtv1.VGPUOptions) *VMBuilder {
	GPUs := v.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs
	gpu := kubevirtv1.GPU{
		Name:              name,
		DeviceName:        hostDeviceName,
		Tag:               tag,
		VirtualGPUOptions: opts,
	}

	if tag != "" {
		gpu.Tag = tag
	}

	GPUs = append(GPUs, gpu)
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs = GPUs
	return v
}

// Adds the specified GPU if it does not yet exist
func (v *VMBuilder) AddGPU(name, hostDeviceName, tag string, opts *kubevirtv1.VGPUOptions) *VMBuilder {
	GPUs := v.VirtualMachine.Spec.Template.Spec.Domain.Devices.GPUs

	found := false
	for _, gpu := range GPUs {
		if gpu.Name == name && gpu.DeviceName == hostDeviceName {
			found = true
		}
	}

	if !found {
		v.GPU(name, hostDeviceName, tag, opts)
	}
	return v
}

func (v *VMBuilder) TPM() *VMBuilder {
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.TPM = &kubevirtv1.TPMDevice{}
	return v
}

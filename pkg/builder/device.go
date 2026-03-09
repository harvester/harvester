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

func (v *VMBuilder) GPU(name, hostDeviceName, tag string, _ *kubevirtv1.VGPUOptions) *VMBuilder {
	return v.HostDevice(name, hostDeviceName, tag)
}

func (v *VMBuilder) TPM() *VMBuilder {
	v.VirtualMachine.Spec.Template.Spec.Domain.Devices.TPM = &kubevirtv1.TPMDevice{}
	return v
}

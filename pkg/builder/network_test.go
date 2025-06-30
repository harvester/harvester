package builder

import "testing"

func TestSetNetworkInterfaceBootOrder(t *testing.T) {
	builder := NewVMBuilder("test")
	builder.NetworkInterface("eth0", "virtio", "00:00:00:00:00:00", NetworkInterfaceTypeBridge, "testnet")
	builder.NetworkInterface("eth1", "virtio", "00:00:00:00:00:01", NetworkInterfaceTypeBridge, "testnet")

	if builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.Interfaces[0].BootOrder != nil {
		t.Error("Boot order set on network interface eth0")
	}

	if builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.Interfaces[1].BootOrder != nil {
		t.Error("Boot order set on network interface eth1")
	}

	builder.SetNetworkInterfaceBootOrder("eth1", 3)

	if builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.Interfaces[0].BootOrder != nil {
		t.Error("Boot order set on network interface eth0")
	}

	if builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.Interfaces[1].BootOrder == nil ||
		*builder.VirtualMachine.Spec.Template.Spec.Domain.Devices.Interfaces[1].BootOrder != 3 {
		t.Error("Failed to set boot order on network interface eth1")
	}
}

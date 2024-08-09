package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

var (
	vm1 = &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							HostDevices: []kubevirtv1.HostDevice{
								{
									Name:       "sample-vf1",
									DeviceName: "intel.com/X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION",
								},
							},
						},
					},
				},
			},
		},
	}

	vm2 = &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							HostDevices: []kubevirtv1.HostDevice{
								{
									Name:       "sample-vf1",
									DeviceName: "intel.com/X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION",
								},
							},
						},
					},
				},
			},
		},
	}

	vm3 = &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							GPUs: []kubevirtv1.GPU{
								{
									Name:       "sample-vgpu",
									DeviceName: "nvidia.com/GRID_A100-20C",
								},
							},
						},
					},
				},
			},
		},
	}

	vm4 = &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							GPUs: []kubevirtv1.GPU{
								{
									Name:       "sample-vgpu",
									DeviceName: "nvidia.com/GRID_A100-10C",
								},
							},
						},
					},
				},
			},
		},
	}

	node1 = &corev1.Node{
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				"nvidia.com/GRID_A100-20C":                            *resource.NewQuantity(2, resource.DecimalSI),
				"intel.com/X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION": *resource.NewQuantity(16, resource.DecimalSI),
			},
		},
	}

	node2 = &corev1.Node{
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				"nvidia.com/GRID_A100-10C":                            *resource.NewQuantity(4, resource.DecimalSI),
				"intel.com/X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION": *resource.NewQuantity(16, resource.DecimalSI),
			},
		},
	}

	nodes = []*corev1.Node{node1, node2}
	vms   = []*kubevirtv1.VirtualMachine{vm1, vm2, vm3, vm4}
)

func Test_calculateAllocation(t *testing.T) {
	assert := require.New(t)
	nodeDeviceAvailability := calculateAllocation(nodes, vms)
	assert.Equal(nodeDeviceAvailability["nvidia.com/GRID_A100-20C"], *resource.NewQuantity(1, resource.DecimalSI), "expect to find 1 A100-20C device")
	assert.Equal(nodeDeviceAvailability["nvidia.com/GRID_A100-10C"], *resource.NewQuantity(3, resource.DecimalSI), "expect to find 3 A100-10C device")
	assert.Equal(nodeDeviceAvailability["intel.com/X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION"], *resource.NewQuantity(30, resource.DecimalSI), "expect to find 30 X540_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION VF's")
}

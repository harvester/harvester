package virtualmachineinstance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/patch"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestPatchMacAddress(t *testing.T) {
	tests := []struct {
		name    string
		vm      *kubevirtv1.VirtualMachine
		vmi     *kubevirtv1.VirtualMachineInstance
		patches types.PatchOps
	}{
		{
			name: "vm without annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{},
			},
			vmi:     &kubevirtv1.VirtualMachineInstance{},
			patches: nil,
		},
		{
			name: "vm without harvesterhci.io/mac-address annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			vmi:     &kubevirtv1.VirtualMachineInstance{},
			patches: nil,
		},
		{
			name: "vm with empty harvesterhci.io/mac-address annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": "",
					},
				},
			},
			vmi:     &kubevirtv1.VirtualMachineInstance{},
			patches: nil,
		},
		{
			name: "vm with invalid harvesterhci.io/mac-address annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": "invalid-json",
					},
				},
			},
			vmi:     &kubevirtv1.VirtualMachineInstance{},
			patches: nil,
		},
		{
			name: "vm with valid harvesterhci.io/mac-address annotation and vm interfaces don't have macaddress",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": `{"default":"00:11:22:33:44:55"}`,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name: "default",
										},
									},
								},
							},
						},
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "default",
								},
							},
						},
					},
				},
			},
			patches: types.PatchOps{`{"op": "add", "path": "/spec/domain/devices/interfaces/0/macAddress", "value": "00:11:22:33:44:55"}`},
		},
		{
			name: "vm with valid harvesterhci.io/mac-address annotation and vm interfaces have macaddress",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": `{"default":"00:11:22:33:44:55"}`,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "default",
											MacAddress: "11:22:33:44:55:66",
										},
									},
								},
							},
						},
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "default",
								},
							},
						},
					},
				},
			},
			patches: types.PatchOps{},
		},
		{
			name: "vm with valid harvesterhci.io/mac-address annotation, but it doesn't have matched name with vm interfaces",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": `{}`,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name: "default",
										},
									},
								},
							},
						},
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "default",
								},
							},
						},
					},
				},
			},
			patches: types.PatchOps{},
		},
		{
			name: "vm with valid harvesterhci.io/mac-address annotation, but matched name entry with empty value",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/mac-address": `{"default":""}`,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name: "default",
										},
									},
								},
							},
						},
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							Interfaces: []kubevirtv1.Interface{
								{
									Name: "default",
								},
							},
						},
					},
				},
			},
			patches: types.PatchOps{},
		},
	}

	for _, tc := range tests {
		clientSet := fake.NewSimpleClientset()
		mutator := NewMutator(fakeclients.VirtualMachineCache(clientSet.KubevirtV1().VirtualMachines))
		patchOps, err := mutator.(*vmiMutator).patchMacAddress(tc.vm, tc.vmi)
		assert.Nil(t, err, tc.name)
		assert.Equal(t, tc.patches, patchOps, tc.name)
	}
}

func TestCreateWithoutVM(t *testing.T) {
	vmi := &kubevirtv1.VirtualMachineInstance{
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					Interfaces: []kubevirtv1.Interface{
						{
							Name: "default",
						},
					},
				},
			},
		},
	}
	req := &types.Request{}
	clientSet := fake.NewSimpleClientset()
	mutator := NewMutator(fakeclients.VirtualMachineCache(clientSet.KubevirtV1().VirtualMachines))
	patchOps, err := mutator.Create(req, vmi)
	assert.Nil(t, err)
	assert.Nil(t, patchOps)
}

const (
	vmiWithHostDevice = `{
    "apiVersion": "kubevirt.io/v1",
    "kind": "VirtualMachineInstance",
    "metadata": {
        "annotations": {
            "harvesterhci.io/sshNames": "[]",
            "kubevirt.io/latest-observed-api-version": "v1",
            "kubevirt.io/storage-observed-api-version": "v1",
            "kubevirt.io/vm-generation": "6"
        },
        "creationTimestamp": "2026-02-13T00:54:43Z",
        "finalizers": [
            "kubevirt.io/virtualMachineControllerFinalize",
            "foregroundDeleteVirtualMachine",
            "wrangler.cattle.io/VMController.BackfillObservedNetworkMacAddress",
            "wrangler.cattle.io/harvester-lb-vmi-controller",
            "wrangler.cattle.io/virtual-machine-deletion"
        ],
        "generation": 10,
        "labels": {
            "harvesterhci.io/vmName": "vf-test",
            "kubevirt.io/nodeName": "dell-190-tink-system"
        },
        "name": "vf-test",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "kubevirt.io/v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "VirtualMachine",
                "name": "vf-test",
                "uid": "b3651358-6f38-4c49-a000-6003fccd5971"
            }
        ],
        "resourceVersion": "23028692",
        "uid": "3b6f6fbe-4ea0-4446-9945-0ceb8e7852ff"
    },
    "spec": {
        "affinity": {
            "nodeAffinity": {
                "requiredDuringSchedulingIgnoredDuringExecution": {
                    "nodeSelectorTerms": [
                        {
                            "matchExpressions": [
                                {
                                    "key": "network.harvesterhci.io/mgmt",
                                    "operator": "In",
                                    "values": [
                                        "true"
                                    ]
                                }
                            ]
                        }
                    ]
                }
            }
        },
        "architecture": "amd64",
        "domain": {
            "cpu": {
                "cores": 8,
                "maxSockets": 1,
                "model": "host-model",
                "sockets": 1,
                "threads": 1
            },
            "devices": {
                "disks": [
                    {
                        "bootOrder": 1,
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "disk-0"
                    },
                    {
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "cloudinitdisk"
                    }
                ],
                "hostDevices": [
                    {
                        "deviceName": "intel.com/82599_ETHERNET_CONTROLLER_VIRTUAL_FUNCTION",
                        "name": "hostname.subdomain"
                    }
                ],
                "inputs": [
                    {
                        "bus": "usb",
                        "name": "tablet",
                        "type": "tablet"
                    }
                ],
                "interfaces": [
                    {
                        "bridge": {},
                        "macAddress": "6a:6c:5e:e1:9f:ba",
                        "model": "virtio",
                        "name": "default"
                    }
                ]
            },
            "features": {
                "acpi": {
                    "enabled": true
                }
            },
            "firmware": {
                "serial": "16ce0679-22dc-4fa0-aed4-320127559198",
                "uuid": "5ad2c0bb-ba8b-4c3f-971c-4d371ecdaffb"
            },
            "machine": {
                "type": "q35"
            },
            "memory": {
                "guest": "16Gi",
                "maxGuest": "64Gi"
            },
            "resources": {
                "limits": {
                    "cpu": "8",
                    "memory": "16Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "16Gi"
                }
            }
        },
        "evictionStrategy": "LiveMigrateIfPossible",
        "hostname": "vf-test",
        "networks": [
            {
                "multus": {
                    "networkName": "default/workload"
                },
                "name": "default"
            }
        ],
        "terminationGracePeriodSeconds": 120,
        "volumes": [
            {
                "name": "disk-0",
                "persistentVolumeClaim": {
                    "claimName": "vf-test-disk-0-dgzjz"
                }
            },
            {
                "cloudInitNoCloud": {
                    "networkDataSecretRef": {
                        "name": "vf-test-t8snk"
                    },
                    "secretRef": {
                        "name": "vf-test-t8snk"
                    }
                },
                "name": "cloudinitdisk"
            }
        ]
    }
}`

	vmiWithoutHostDevice = `{
    "apiVersion": "kubevirt.io/v1",
    "kind": "VirtualMachineInstance",
    "metadata": {
        "annotations": {
            "harvesterhci.io/sshNames": "[]",
            "kubevirt.io/latest-observed-api-version": "v1",
            "kubevirt.io/storage-observed-api-version": "v1",
            "kubevirt.io/vm-generation": "6"
        },
        "creationTimestamp": "2026-02-13T00:54:43Z",
        "finalizers": [
            "kubevirt.io/virtualMachineControllerFinalize",
            "foregroundDeleteVirtualMachine",
            "wrangler.cattle.io/VMController.BackfillObservedNetworkMacAddress",
            "wrangler.cattle.io/harvester-lb-vmi-controller",
            "wrangler.cattle.io/virtual-machine-deletion"
        ],
        "generation": 10,
        "labels": {
            "harvesterhci.io/vmName": "vf-test",
            "kubevirt.io/nodeName": "dell-190-tink-system"
        },
        "name": "vf-test",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "kubevirt.io/v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "VirtualMachine",
                "name": "vf-test",
                "uid": "b3651358-6f38-4c49-a000-6003fccd5971"
            }
        ],
        "resourceVersion": "23028692",
        "uid": "3b6f6fbe-4ea0-4446-9945-0ceb8e7852ff"
    },
    "spec": {
        "affinity": {
            "nodeAffinity": {
                "requiredDuringSchedulingIgnoredDuringExecution": {
                    "nodeSelectorTerms": [
                        {
                            "matchExpressions": [
                                {
                                    "key": "network.harvesterhci.io/mgmt",
                                    "operator": "In",
                                    "values": [
                                        "true"
                                    ]
                                }
                            ]
                        }
                    ]
                }
            }
        },
        "architecture": "amd64",
        "domain": {
            "cpu": {
                "cores": 8,
                "maxSockets": 1,
                "model": "host-model",
                "sockets": 1,
                "threads": 1
            },
            "devices": {
                "disks": [
                    {
                        "bootOrder": 1,
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "disk-0"
                    },
                    {
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "cloudinitdisk"
                    }
                ],
                "inputs": [
                    {
                        "bus": "usb",
                        "name": "tablet",
                        "type": "tablet"
                    }
                ],
                "interfaces": [
                    {
                        "bridge": {},
                        "macAddress": "6a:6c:5e:e1:9f:ba",
                        "model": "virtio",
                        "name": "default"
                    }
                ]
            },
            "features": {
                "acpi": {
                    "enabled": true
                }
            },
            "firmware": {
                "serial": "16ce0679-22dc-4fa0-aed4-320127559198",
                "uuid": "5ad2c0bb-ba8b-4c3f-971c-4d371ecdaffb"
            },
            "machine": {
                "type": "q35"
            },
            "memory": {
                "guest": "16Gi",
                "maxGuest": "64Gi"
            },
            "resources": {
                "limits": {
                    "cpu": "8",
                    "memory": "16Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "16Gi"
                }
            }
        },
        "evictionStrategy": "LiveMigrateIfPossible",
        "hostname": "vf-test",
        "networks": [
            {
                "multus": {
                    "networkName": "default/workload"
                },
                "name": "default"
            }
        ],
        "terminationGracePeriodSeconds": 120,
        "volumes": [
            {
                "name": "disk-0",
                "persistentVolumeClaim": {
                    "claimName": "vf-test-disk-0-dgzjz"
                }
            },
            {
                "cloudInitNoCloud": {
                    "networkDataSecretRef": {
                        "name": "vf-test-t8snk"
                    },
                    "secretRef": {
                        "name": "vf-test-t8snk"
                    }
                },
                "name": "cloudinitdisk"
            }
        ]
    }
}`

	vmiWithGPU = `{
    "apiVersion": "kubevirt.io/v1",
    "kind": "VirtualMachineInstance",
    "metadata": {
        "annotations": {
            "harvesterhci.io/sshNames": "[]",
            "kubevirt.io/latest-observed-api-version": "v1",
            "kubevirt.io/storage-observed-api-version": "v1",
            "kubevirt.io/vm-generation": "6"
        },
        "creationTimestamp": "2026-02-13T00:54:43Z",
        "finalizers": [
            "kubevirt.io/virtualMachineControllerFinalize",
            "foregroundDeleteVirtualMachine",
            "wrangler.cattle.io/VMController.BackfillObservedNetworkMacAddress",
            "wrangler.cattle.io/harvester-lb-vmi-controller",
            "wrangler.cattle.io/virtual-machine-deletion"
        ],
        "generation": 10,
        "labels": {
            "harvesterhci.io/vmName": "vf-test",
            "kubevirt.io/nodeName": "dell-190-tink-system"
        },
        "name": "vf-test",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "kubevirt.io/v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "VirtualMachine",
                "name": "vf-test",
                "uid": "b3651358-6f38-4c49-a000-6003fccd5971"
            }
        ],
        "resourceVersion": "23028692",
        "uid": "3b6f6fbe-4ea0-4446-9945-0ceb8e7852ff"
    },
    "spec": {
        "affinity": {
            "nodeAffinity": {
                "requiredDuringSchedulingIgnoredDuringExecution": {
                    "nodeSelectorTerms": [
                        {
                            "matchExpressions": [
                                {
                                    "key": "network.harvesterhci.io/mgmt",
                                    "operator": "In",
                                    "values": [
                                        "true"
                                    ]
                                }
                            ]
                        }
                    ]
                }
            }
        },
        "architecture": "amd64",
        "domain": {
            "cpu": {
                "cores": 8,
                "maxSockets": 1,
                "model": "host-model",
                "sockets": 1,
                "threads": 1
            },
            "devices": {
                "disks": [
                    {
                        "bootOrder": 1,
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "disk-0"
                    },
                    {
                        "disk": {
                            "bus": "virtio"
                        },
                        "name": "cloudinitdisk"
                    }
                ],
                "gpus": [
                    {
                        "deviceName": "nvidia.com/GRID_A100-1-10C",
                        "name": "hostname.subdomain"
                    }
                ],
                "inputs": [
                    {
                        "bus": "usb",
                        "name": "tablet",
                        "type": "tablet"
                    }
                ],
                "interfaces": [
                    {
                        "bridge": {},
                        "macAddress": "6a:6c:5e:e1:9f:ba",
                        "model": "virtio",
                        "name": "default"
                    }
                ]
            },
            "features": {
                "acpi": {
                    "enabled": true
                }
            },
            "firmware": {
                "serial": "16ce0679-22dc-4fa0-aed4-320127559198",
                "uuid": "5ad2c0bb-ba8b-4c3f-971c-4d371ecdaffb"
            },
            "machine": {
                "type": "q35"
            },
            "memory": {
                "guest": "16Gi",
                "maxGuest": "64Gi"
            },
            "resources": {
                "limits": {
                    "cpu": "8",
                    "memory": "16Gi"
                },
                "requests": {
                    "cpu": "500m",
                    "memory": "16Gi"
                }
            }
        },
        "evictionStrategy": "LiveMigrateIfPossible",
        "hostname": "vf-test",
        "networks": [
            {
                "multus": {
                    "networkName": "default/workload"
                },
                "name": "default"
            }
        ],
        "terminationGracePeriodSeconds": 120,
        "volumes": [
            {
                "name": "disk-0",
                "persistentVolumeClaim": {
                    "claimName": "vf-test-disk-0-dgzjz"
                }
            },
            {
                "cloudInitNoCloud": {
                    "networkDataSecretRef": {
                        "name": "vf-test-t8snk"
                    },
                    "secretRef": {
                        "name": "vf-test-t8snk"
                    }
                },
                "name": "cloudinitdisk"
            }
        ]
    }
}`
)

// Test_patchDeviceNames validates that `name` field will be base32 encoded for both HostDevice and GPU device
func Test_patchDeviceName(t *testing.T) {
	var testCases = []struct {
		name          string
		vmi           string
		expectedPatch bool
	}{
		{
			name:          "vmi with hostdevice",
			vmi:           vmiWithHostDevice,
			expectedPatch: true,
		},
		{
			name:          "vmi without hostdevice",
			vmi:           vmiWithoutHostDevice,
			expectedPatch: false,
		},
		{
			name:          "vmi without hostdevice",
			vmi:           vmiWithGPU,
			expectedPatch: true,
		},
	}

	assert := assert.New(t)
	for _, tc := range testCases {
		vmi, err := generateVMI([]byte(tc.vmi))
		assert.NoError(err, "should unmarshal vmi json without error", tc.name)
		patchOps, err := patchDeviceName(vmi)
		assert.NoError(err, "expected no error during patchOps", tc.name)
		if tc.expectedPatch {
			assert.NotEmpty(patchOps, "expected to find patchOps", tc.name)
			patchData := fmt.Sprintf("[%s]", strings.Join(patchOps, ","))
			patchedVMIBytes, err := patch.Apply([]byte(tc.vmi), []byte(patchData))
			assert.NoError(err, "expected no error during application of patch to vmi", tc.name)
			patchedVMI, err := generateVMI(patchedVMIBytes)
			assert.NoError(err, "expected no error during generation of patched vmi", tc.name)
			for i, device := range patchedVMI.Spec.Domain.Devices.HostDevices {
				originalDevice := vmi.Spec.Domain.Devices.HostDevices[i]
				assert.Equal(device.Name, generateEncodedAlias(originalDevice.Name), "expected generated device name to match", tc.name)
			}
			for i, device := range patchedVMI.Spec.Domain.Devices.GPUs {
				originalDevice := vmi.Spec.Domain.Devices.GPUs[i]
				assert.Equal(device.Name, generateEncodedAlias(originalDevice.Name), "expected generated device name to match", tc.name)
			}
		}
	}
}

func generateVMI(input []byte) (*kubevirtv1.VirtualMachineInstance, error) {
	vmi := &kubevirtv1.VirtualMachineInstance{}
	err := json.Unmarshal(input, vmi)
	return vmi, err
}

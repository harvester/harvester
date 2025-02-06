package virtualmachine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_virtualMachineValidator_duplicateMacAddress(t *testing.T) {
	tests := []struct {
		name        string
		vm          *kubevirtv1.VirtualMachine
		expectError bool
	}{
		{
			name: "duplicate mac in different L2,returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:02",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "duplicate mac in same L2,returns error",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-5",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-5",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "same vm name with same mac address duirng vm migration,vm restore,returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Update case - add a new interface to an existing vm without mac address, returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
								{
									Name: "nic-2",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:01",
										},
										{
											Name: "nic-2",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Update case - add a new interface to an existing vm with conflicting mac address, returns error",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
								{
									Name: "nic-2",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:01",
										},
										{
											Name:       "nic-2",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Update case - modify the mac address in an existing vm,returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:02",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "duplicate mac address in same L2 with nad in different namespace,returns error",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "non-default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "duplicate mac address in different L2 with nad in different namespace,returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "non-default/vlan-2",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "00:00:00:00:00:03",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "duplicate mac in same L2 different nad,returns error",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-4",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-4",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-4",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "same vm name and same mac n different interface,same L2, different mac, returns error",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-6",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-6",
											MacAddress: "00:00:00:00:00:01",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "empty mac, returns success",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: "default/vlan-1",
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name:       "nic-1",
											MacAddress: "",
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	label1 := make(map[string]string)
	label1[keyClusterNetwork] = "cluster-1"

	label2 := make(map[string]string)
	label2[keyClusterNetwork] = "cluster-2"

	label3 := make(map[string]string)
	label3[keyClusterNetwork] = "cluster-3"

	label4 := make(map[string]string)
	label4[keyClusterNetwork] = "cluster-1"

	existingNADs := []*cniv1.NetworkAttachmentDefinition{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-1",
				Namespace: "default",
				Labels:    label1,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-2",
				Namespace: "default",
				Labels:    label2,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-3",
				Namespace: "default",
				Labels:    label3,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-4",
				Namespace: "default",
				Labels:    label4,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-1",
				Namespace: "non-default",
				Labels:    label1,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan-2",
				Namespace: "non-default",
				Labels:    label2,
			},
		},
	}

	existingVMs := []*kubevirtv1.VirtualMachine{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm1",
				Namespace: "default",
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Networks: []kubevirtv1.Network{
							{
								Name: "nic-1",
								NetworkSource: kubevirtv1.NetworkSource{
									Multus: &kubevirtv1.MultusNetwork{
										NetworkName: "default/vlan-1",
									},
								},
							},
						},
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								Interfaces: []kubevirtv1.Interface{
									{
										Name:       "nic-1",
										MacAddress: "00:00:00:00:00:01",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm2",
				Namespace: "default",
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Networks: []kubevirtv1.Network{
							{
								Name: "nic-2",
								NetworkSource: kubevirtv1.NetworkSource{
									Multus: &kubevirtv1.MultusNetwork{
										NetworkName: "default/vlan-2",
									},
								},
							},
						},
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								Interfaces: []kubevirtv1.Interface{
									{
										Name:       "nic-2",
										MacAddress: "00:00:00:00:00:02",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm3",
				Namespace: "default",
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Networks: []kubevirtv1.Network{
							{
								Name: "nic-3",
								NetworkSource: kubevirtv1.NetworkSource{
									Multus: &kubevirtv1.MultusNetwork{
										NetworkName: "default/vlan-3",
									},
								},
							},
						},
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								Interfaces: []kubevirtv1.Interface{
									{
										Name:       "nic-3",
										MacAddress: "00:00:00:00:00:03",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Networks: []kubevirtv1.Network{
							{
								Name: "nic-1",
								NetworkSource: kubevirtv1.NetworkSource{
									Multus: &kubevirtv1.MultusNetwork{
										NetworkName: "default/vlan-1",
									},
								},
							},
						},
						Domain: kubevirtv1.DomainSpec{
							Devices: kubevirtv1.Devices{
								Interfaces: []kubevirtv1.Interface{
									{
										Name:       "nic-1",
										MacAddress: "",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var clientset = fake.NewSimpleClientset()
	for _, existingVM := range existingVMs {
		var err = clientset.Tracker().Add(existingVM)
		assert.Nil(t, err, "mock resource should add into fake controller tracker")
	}

	nadGvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}

	for _, existingNAD := range existingNADs {
		if err := clientset.Tracker().Create(nadGvr, existingNAD.DeepCopy(), existingNAD.Namespace); err != nil {
			t.Fatalf("failed to add nad %+v", existingNAD)
		}
	}

	fakeVMCache := fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines)
	fakeNadCache := fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)

	validator := NewValidator(nil, nil, nil, nil, nil, nil, fakeVMCache, nil, fakeNadCache, nil).(*vmValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.checkForDuplicateMacAddrs(tc.vm)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

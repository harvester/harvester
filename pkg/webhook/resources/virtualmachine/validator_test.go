package virtualmachine

import (
	"testing"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckMaintenanceModeStrategyIsValid(t *testing.T) {
	var testCases = []struct {
		name        string
		expectError bool
		oldVM       *kubevirtv1.VirtualMachine
		newVM       *kubevirtv1.VirtualMachine
	}{
		{
			name:        "accept new VM if maintenance mode strategy label is not set",
			expectError: false,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "reject new VM if maintenance mode strategy label is set to invalid value",
			expectError: true,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "reject update to VM if maintenance mode strategy label is invalid for new VM",
			expectError: true,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyMigrate,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			// This case is crucial, so Harvester can still operate existing VMs with
			// bogus maintenance-mode strategies (i.e. update their status, shut them
			// down, etc.)
			name:        "accept update to VM if maintenance mode strategy label was invalid on old VM",
			expectError: false,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "accept update without maintenance mode strategy label, if VM did not have one before",
			expectError: false,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			// This case ensures that IF the maintenance-mode label is updated, it is
			// updated with a valid value
			name:        "reject update to VM with invalid maintenance-mode strategy label, even if maintenance mode strategy label was invalid on old VM",
			expectError: true,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "barfoo",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "accept removal of maintenance mode strategy label",
			expectError: false,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "migrate",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "accept new VM maintenance mode strategy label is set to valid value",
			expectError: false,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyMigrate,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
	}

	validator := NewValidator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).(*vmValidator)

	for _, tc := range testCases {
		err := validator.checkMaintenanceModeStrategyIsValid(tc.newVM, tc.oldVM)
		if tc.expectError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}

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

	validator := NewValidator(nil, nil, nil, nil, nil, nil, fakeVMCache, nil, fakeNadCache, nil, nil, nil).(*vmValidator)

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

func TestVmValidator_Update(t *testing.T) {
	templateVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				"harvesterhci.io/volumeClaimTemplates": `[{"metadata":{"name":"test-disk-0",` +
					`"annotations":{"harvesterhci.io/imageId":"default/image"}},` +
					`"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}}` +
					`,"volumeMode":"Block","storageClassName":"longhorn-image"}}]`,
			},
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
						Memory: &kubevirtv1.Memory{
							Guest: resource.NewQuantity(1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		oldVM         *kubevirtv1.VirtualMachine
		newVM         *kubevirtv1.VirtualMachine
		oldObjMeta    *metav1.ObjectMeta
		newObjMeta    *metav1.ObjectMeta
		newSpec       *kubevirtv1.VirtualMachineSpec
		expectedError bool
	}{
		{
			name:  "storage class name is changed which results in rejection",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": `[{"metadata":{"name":"test-disk-0",` +
						`"annotations":{"harvesterhci.io/imageId":"default/image"}},` +
						`"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}}` +
						`,"volumeMode":"Block","storageClassName":"longhorn"}}]`,
				},
			},
			newSpec:       nil,
			expectedError: true,
		},
		{
			name:  "annotation removed resulting in success",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
			},
			newSpec:       nil,
			expectedError: false,
		},
		{
			name:  "annotation added resulting in success",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": `[{"metadata":{"name":"test-disk-0"` +
						`,"annotations":{"harvesterhci.io/imageId":"default/image"}},"spec":{"accessModes"` +
						`:["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}},"volumeMode":` +
						`"Block","storageClassName":"longhorn-image"}},{"metadata":{"name":"test-disk-1",` +
						`"annotations":{"harvesterhci.io/imageId":"default/image"}},"spec":{"accessModes":` +
						`["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}},"volumeMode":` +
						`"Block","storageClassName":"longhorn"}}]`,
				},
			},
			newSpec:       nil,
			expectedError: false,
		},
		{
			name:  "annotations with bad json are rejected",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": `[{"]`,
				},
			},
			newSpec:       nil,
			expectedError: true,
		},
		{
			name:  "empty annotation is allowed on both objects",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			oldObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": "",
				},
			},
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": "",
				},
			},
			newSpec:       nil,
			expectedError: false,
		},
		{
			name:  "nil storage class name is handled properly",
			oldVM: templateVM.DeepCopy(),
			newVM: templateVM.DeepCopy(),
			newObjMeta: &metav1.ObjectMeta{
				Name:      templateVM.Name,
				Namespace: templateVM.Namespace,
				Annotations: map[string]string{
					"harvesterhci.io/volumeClaimTemplates": `[{"metadata":{"name":"test-disk-0"` +
						`,"annotations":{"harvesterhci.io/imageId":"default/image"}},"spec":{"accessModes"` +
						`:["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}},"volumeMode":` +
						`"Block"}},{"metadata":{"name":"test-disk-1",` +
						`"annotations":{"harvesterhci.io/imageId":"default/image"}},"spec":{"accessModes":` +
						`["ReadWriteMany"],"resources":{"requests":{"storage":"10Gi"}},"volumeMode":` +
						`"Block"}}]`,
				},
			},
			newSpec:       nil,
			expectedError: false,
		},
	}

	corefakeclientset := corefake.NewClientset()
	err := corefakeclientset.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: templateVM.Namespace,
	}})
	assert.NoError(t, err)
	fakeNSCache := fakeclients.NamespaceCache(corefakeclientset.CoreV1().Namespaces)
	validator := NewValidator(fakeNSCache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).(*vmValidator)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.oldObjMeta != nil {
				test.oldVM.ObjectMeta = *test.oldObjMeta
			}
			if test.newObjMeta != nil {
				test.newVM.ObjectMeta = *test.newObjMeta
			}
			if test.newSpec != nil {
				test.newVM.Spec = *test.newSpec
			}
			err := validator.Update(nil, test.oldVM, test.newVM)

			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckCdRomVolumeIsValid(t *testing.T) {
	var testCases = []struct {
		name        string
		expectError bool
		vm       *kubevirtv1.VirtualMachine
	}{
			{
				name:        "accept VM with empty cdrom connected via SATA",
				expectError: false,
				vm: &kubevirtv1.VirtualMachine{
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name: "cd1",
												DiskDevice: kubevirtv1.DiskDevice{
													CDRom: &kubevirtv1.CDRomTarget{
														Bus: kubevirtv1.DiskBusSATA,
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{},
							},
						},
					},
				},
			},
			{
				name:        "reject VM with empty cdrom connected via SCSI",
				expectError: true,
				vm: &kubevirtv1.VirtualMachine{
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name: "cd1",
												DiskDevice: kubevirtv1.DiskDevice{
													CDRom: &kubevirtv1.CDRomTarget{
														Bus: kubevirtv1.DiskBusSCSI,
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{},
							},
						},
					},
				},
			},
			{
				name:        "accept VM with hotpluggable cdrom volume connected via SATA",
				expectError: false,
				vm: &kubevirtv1.VirtualMachine{
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name: "cd1",
												DiskDevice: kubevirtv1.DiskDevice{
													CDRom: &kubevirtv1.CDRomTarget{
														Bus: kubevirtv1.DiskBusSATA,
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									kubevirtv1.Volume{
										Name: "cd1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "cd1-pvc",
												},
												Hotpluggable: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				name:        "reject VM with hotpluggable cdrom volume connected via SCSI",
				expectError: true,
				vm: &kubevirtv1.VirtualMachine{
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name: "cd1",
												DiskDevice: kubevirtv1.DiskDevice{
													CDRom: &kubevirtv1.CDRomTarget{
														Bus: kubevirtv1.DiskBusSCSI,
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									kubevirtv1.Volume{
										Name: "cd1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "cd1-pvc",
												},
												Hotpluggable: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

	validator := NewValidator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).(*vmValidator)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.checkCdRomVolumeIsValid(tc.vm)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

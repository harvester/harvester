package virtualmachineinstance

import (
	"testing"

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

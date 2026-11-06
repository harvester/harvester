package virtualmachineinstance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestVMIValidatorCreate(t *testing.T) {
	persistentTrue := true

	tests := []struct {
		name        string
		vmi         *kubevirtv1.VirtualMachineInstance
		scExists    bool
		expectError bool
	}{
		{
			name: "persistent TPM, SC missing → reject",
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							TPM: &kubevirtv1.TPMDevice{Persistent: &persistentTrue},
						},
					},
				},
			},
			scExists:    false,
			expectError: true,
		},
		{
			name: "persistent EFI, SC missing → reject",
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Firmware: &kubevirtv1.Firmware{
							Bootloader: &kubevirtv1.Bootloader{
								EFI: &kubevirtv1.EFI{Persistent: &persistentTrue},
							},
						},
					},
				},
			},
			scExists:    false,
			expectError: true,
		},
		{
			name: "persistent TPM, SC exists → accept",
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							TPM: &kubevirtv1.TPMDevice{Persistent: &persistentTrue},
						},
					},
				},
			},
			scExists:    true,
			expectError: false,
		},
		{
			name: "no TPM or EFI, SC missing → accept",
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{},
				},
			},
			scExists:    false,
			expectError: false,
		},
		{
			name: "non-persistent TPM, SC missing → accept",
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							TPM: &kubevirtv1.TPMDevice{},
						},
					},
				},
			},
			scExists:    false,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.scExists {
				assert.NoError(t, clientset.Tracker().Add(&storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{Name: util.StorageClassVmstatePersistence},
				}))
			}
			v := NewValidator(fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses))
			err := v.Create(&types.Request{}, tc.vmi)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

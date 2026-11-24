package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCleanupOrphanedPCIDevicesFinalizer(t *testing.T) {
	type input struct {
		addon *harvesterv1.Addon
		vmis  []*kubevirtv1.VirtualMachineInstance
	}
	type output struct {
		// finalizers keyed by namespace/name after cleanup
		finalizersAfter map[string][]string
		err             error
	}

	finalizerOnly := []string{pcidevicesVMIFinalizer}
	finalizerWithOthers := []string{"kubevirt.io/virtualMachineControllerFinalize", pcidevicesVMIFinalizer}

	tests := []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil addon",
			given: input{
				addon: nil,
				vmis:  nil,
			},
			expected: output{finalizersAfter: map[string][]string{}, err: nil},
		},
		{
			name: "ignore non-pcidevices addon",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: "other-addon", Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDisabled},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "vm1", Namespace: "default",
							Finalizers: finalizerOnly,
						},
					},
				},
			},
			expected: output{finalizersAfter: map[string][]string{"default/vm1": finalizerOnly}, err: nil},
		},
		{
			name: "ignore pcidevices addon that is not disabled",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: pcidevicesControllerAddonName, Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDeployed},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "vm1", Namespace: "default",
							Finalizers: finalizerOnly,
						},
					},
				},
			},
			expected: output{finalizersAfter: map[string][]string{"default/vm1": finalizerOnly}, err: nil},
		},
		{
			name: "remove finalizer from VMI without devices when addon disabled",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: pcidevicesControllerAddonName, Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDisabled},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "plain-vm", Namespace: "default",
							Finalizers: finalizerWithOthers,
						},
					},
				},
			},
			expected: output{
				finalizersAfter: map[string][]string{
					"default/plain-vm": {"kubevirt.io/virtualMachineControllerFinalize"},
				},
				err: nil,
			},
		},
		{
			name: "keep finalizer on VMI with host devices when addon disabled",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: pcidevicesControllerAddonName, Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDisabled},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pci-vm", Namespace: "default",
							Finalizers: finalizerOnly,
						},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									HostDevices: []kubevirtv1.HostDevice{{Name: "dev0", DeviceName: "example.com/dev"}},
								},
							},
						},
					},
				},
			},
			expected: output{finalizersAfter: map[string][]string{"default/pci-vm": finalizerOnly}, err: nil},
		},
		{
			name: "keep finalizer on VMI with GPUs when addon disabled",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: pcidevicesControllerAddonName, Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDisabled},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gpu-vm", Namespace: "default",
							Finalizers: finalizerOnly,
						},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									GPUs: []kubevirtv1.GPU{{Name: "gpu0", DeviceName: "nvidia.com/gpu"}},
								},
							},
						},
					},
				},
			},
			expected: output{finalizersAfter: map[string][]string{"default/gpu-vm": finalizerOnly}, err: nil},
		},
		{
			name: "mixed VMIs: remove orphan, keep device VMIs",
			given: input{
				addon: &harvesterv1.Addon{
					ObjectMeta: metav1.ObjectMeta{Name: pcidevicesControllerAddonName, Namespace: "harvester-system"},
					Status:     harvesterv1.AddonStatus{Status: harvesterv1.AddonDisabled},
				},
				vmis: []*kubevirtv1.VirtualMachineInstance{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "plain-vm", Namespace: "default",
							Finalizers: finalizerOnly,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pci-vm", Namespace: "default",
							Finalizers: finalizerOnly,
						},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									HostDevices: []kubevirtv1.HostDevice{{Name: "dev0", DeviceName: "example.com/dev"}},
								},
							},
						},
					},
				},
			},
			expected: output{
				finalizersAfter: map[string][]string{
					"default/plain-vm": nil,
					"default/pci-vm":   finalizerOnly,
				},
				err: nil,
			},
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fake.NewSimpleClientset()
			for _, vmi := range tc.given.vmis {
				_, err := clientSet.KubevirtV1().VirtualMachineInstances(vmi.Namespace).Create(
					nil, vmi, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			h := &Handler{
				vmiCache:  fakeclients.VirtualMachineInstanceCache(clientSet.KubevirtV1().VirtualMachineInstances),
				vmiClient: fakeclients.VirtualMachineInstanceClient(clientSet.KubevirtV1().VirtualMachineInstances),
			}

			_, err := h.cleanupOrphanedPCIDevicesFinalizer("", tc.given.addon)
			assert.Equal(tc.expected.err, err, tc.name)

			for key, expectedFinalizers := range tc.expected.finalizersAfter {
				ns, name := splitKey(key)
				vmi, err := clientSet.KubevirtV1().VirtualMachineInstances(ns).Get(nil, name, metav1.GetOptions{})
				require.NoError(t, err)
				assert.Equal(expectedFinalizers, vmi.Finalizers, "%s finalizers for %s", tc.name, key)
			}
		})
	}
}

func splitKey(key string) (string, string) {
	for i, c := range key {
		if c == '/' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

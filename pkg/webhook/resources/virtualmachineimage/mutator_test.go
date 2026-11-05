package virtualmachineimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVirtualMachineImageMutatorDefaultsBackendFromSkipLonghornDefaultDisk(t *testing.T) {
	tests := []struct {
		name    string
		setting *harvesterv1.Setting
		want    harvesterv1.VMIBackend
	}{
		{
			name: "uses CDI when skip setting value is true",
			setting: &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
				Default:    "false",
				Value:      "true",
			},
			want: harvesterv1.VMIBackendCDI,
		},
		{
			name: "uses CDI when skip setting default is true",
			setting: &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
				Default:    "true",
			},
			want: harvesterv1.VMIBackendCDI,
		},
		{
			name: "uses BackingImage when skip setting is false",
			setting: &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
				Default:    "false",
			},
			want: harvesterv1.VMIBackendBackingImage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(defaultStorageClass(), tc.setting)
			mutator := NewMutator(
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			)
			vmi := newVirtualMachineImage("")

			_, err := mutator.Create(nil, vmi)

			require.NoError(t, err)
			assert.Equal(t, tc.want, vmi.Spec.Backend)
		})
	}
}

func TestVirtualMachineImageMutatorAddsBackendWhenItIsOmitted(t *testing.T) {
	clientset := fake.NewSimpleClientset(defaultStorageClass(), &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
		Default:    "false",
		Value:      "true",
	})
	mutator := NewMutator(
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
	)

	patchOps, err := mutator.Create(nil, newVirtualMachineImage(""))

	require.NoError(t, err)
	assert.Contains(t, patchOps, `{"op": "add", "path": "/spec/backend", "value": "cdi"}`)
}

func TestVirtualMachineImageMutatorPreservesExplicitBackend(t *testing.T) {
	for _, backend := range []harvesterv1.VMIBackend{
		harvesterv1.VMIBackendBackingImage,
		harvesterv1.VMIBackendCDI,
	} {
		t.Run(string(backend), func(t *testing.T) {
			clientset := fake.NewSimpleClientset(defaultStorageClass(), &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
				Default:    "true",
			})
			mutator := NewMutator(
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
			)
			vmi := newVirtualMachineImage(backend)

			_, err := mutator.Create(nil, vmi)

			require.NoError(t, err)
			assert.Equal(t, backend, vmi.Spec.Backend)
		})
	}
}

func TestVirtualMachineImageMutatorRejectsMissingSkipLonghornDefaultDiskSetting(t *testing.T) {
	clientset := fake.NewSimpleClientset(defaultStorageClass())
	mutator := NewMutator(
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
	)

	_, err := mutator.Create(nil, newVirtualMachineImage(""))

	require.Error(t, err)
	assert.Contains(t, err.Error(), settings.SkipLonghornDefaultDiskSettingName)
}

func TestVirtualMachineImageMutatorRejectsInvalidSkipLonghornDefaultDiskSetting(t *testing.T) {
	clientset := fake.NewSimpleClientset(defaultStorageClass(), &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.SkipLonghornDefaultDiskSettingName},
		Default:    "invalid",
	})
	mutator := NewMutator(
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
	)

	_, err := mutator.Create(nil, newVirtualMachineImage(""))

	require.Error(t, err)
	assert.Contains(t, err.Error(), settings.SkipLonghornDefaultDiskSettingName)
}

func newVirtualMachineImage(backend harvesterv1.VMIBackend) *harvesterv1.VirtualMachineImage {
	return &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image",
			Namespace: "default",
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			Backend:     backend,
			DisplayName: "image",
		},
	}
}

func defaultStorageClass() *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
		Provisioner: "driver.longhorn.io",
	}
}

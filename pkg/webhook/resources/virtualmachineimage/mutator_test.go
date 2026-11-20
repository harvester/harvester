package virtualmachineimage

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestVirtualMachineImageMutatorDefaultsBackendFromDefaultStorageClass(t *testing.T) {
	tests := []struct {
		name    string
		scs     []*storagev1.StorageClass
		want    harvesterv1.VMIBackend
		wantErr string
	}{
		{
			name: "uses BackingImage when the default SC is Longhorn without dataEngine",
			scs:  []*storagev1.StorageClass{newSC("lh", true, longhornProvisioner, "")},
			want: harvesterv1.VMIBackendBackingImage,
		},
		{
			name: "uses BackingImage when the default SC is Longhorn v1",
			scs:  []*storagev1.StorageClass{newSC("lh", true, longhornProvisioner, "v1")},
			want: harvesterv1.VMIBackendBackingImage,
		},
		{
			name: "uses CDI when the default SC is Longhorn v2",
			scs:  []*storagev1.StorageClass{newSC("lh2", true, longhornProvisioner, "v2")},
			want: harvesterv1.VMIBackendCDI,
		},
		{
			name: "uses CDI when the default SC is a third-party CSI",
			scs:  []*storagev1.StorageClass{newSC("ext", true, "ext.csi.example.com", "")},
			want: harvesterv1.VMIBackendCDI,
		},
		{
			// CDI then rejects the image, same as an explicit CDI image without a default SC.
			name:    "picks CDI and is rejected when there is no default SC",
			scs:     []*storagev1.StorageClass{newSC("lh", false, longhornProvisioner, "")},
			want:    harvesterv1.VMIBackendCDI,
			wantErr: "missing default storage class",
		},
		{
			name: "ignores non-default Longhorn v1 SCs",
			scs: []*storagev1.StorageClass{
				newSC("lh", false, longhornProvisioner, ""),
				newSC("ext", true, "ext.csi.example.com", ""),
			},
			want: harvesterv1.VMIBackendCDI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vmi := newVirtualMachineImage("")

			original := vmi.DeepCopy()
			patchOps, err := newTestMutator(t, tc.scs...).Create(nil, vmi)

			assert.Equal(t, original, vmi)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, applyImagePatches(t, vmi, patchOps).Spec.Backend)
		})
	}
}

func TestVirtualMachineImageMutatorAddsBackendWhenItIsOmitted(t *testing.T) {
	patchOps, err := newTestMutator(t, cdiDefaultSC()).Create(nil, newVirtualMachineImage(""))

	require.NoError(t, err)
	assert.Contains(t, patchOps, `{"op": "add", "path": "/spec/backend", "value": "cdi"}`)
}

func TestVirtualMachineImageMutatorPreservesExplicitBackend(t *testing.T) {
	for _, backend := range []harvesterv1.VMIBackend{
		harvesterv1.VMIBackendBackingImage,
		harvesterv1.VMIBackendCDI,
	} {
		t.Run(string(backend), func(t *testing.T) {
			vmi := newVirtualMachineImage(backend)

			patchOps, err := newTestMutator(t, cdiDefaultSC()).Create(nil, vmi)

			require.NoError(t, err)
			assert.Equal(t, backend, vmi.Spec.Backend)
			assert.NotContains(t, strings.Join(patchOps, ""), `"/spec/backend"`)
		})
	}
}

func TestVirtualMachineImageMutatorHandlesOmittedBackendOnUpdate(t *testing.T) {
	tests := []struct {
		name       string
		defaultSC  *storagev1.StorageClass
		oldBackend harvesterv1.VMIBackend
		want       harvesterv1.VMIBackend
	}{
		{
			name:       "preserves backingImage when cluster default is CDI",
			defaultSC:  cdiDefaultSC(),
			oldBackend: harvesterv1.VMIBackendBackingImage,
			want:       harvesterv1.VMIBackendBackingImage,
		},
		{
			name:       "preserves CDI when cluster default is backingImage",
			defaultSC:  newSC("lh", true, longhornProvisioner, ""),
			oldBackend: harvesterv1.VMIBackendCDI,
			want:       harvesterv1.VMIBackendCDI,
		},
		{
			name:      "uses cluster default when both backends are empty",
			defaultSC: cdiDefaultSC(),
			want:      harvesterv1.VMIBackendCDI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newVMI := newVirtualMachineImage("")

			oldVMI := newVirtualMachineImage(tc.oldBackend)
			patchOps, err := newTestMutator(t, tc.defaultSC).Update(nil, oldVMI, newVMI)

			require.NoError(t, err)
			assert.Empty(t, newVMI.Spec.Backend)
			assert.Equal(t, tc.oldBackend, oldVMI.Spec.Backend)
			assert.Contains(t, patchOps, fmt.Sprintf(`{"op": "add", "path": "/spec/backend", "value": "%s"}`, tc.want))
			assert.Equal(t, tc.want, applyImagePatches(t, newVMI, patchOps).Spec.Backend)
		})
	}
}

const longhornProvisioner = "driver.longhorn.io"

func applyImagePatches(t *testing.T, vmi *harvesterv1.VirtualMachineImage, patchOps types.PatchOps) *harvesterv1.VirtualMachineImage {
	t.Helper()
	data, err := json.Marshal(vmi)
	require.NoError(t, err)
	patches, err := jsonpatch.DecodePatch([]byte("[" + strings.Join(patchOps, ",") + "]"))
	require.NoError(t, err)
	data, err = patches.Apply(data)
	require.NoError(t, err)
	var patched harvesterv1.VirtualMachineImage
	require.NoError(t, json.Unmarshal(data, &patched))
	return &patched
}

func newTestMutator(t *testing.T, scs ...*storagev1.StorageClass) types.Mutator {
	t.Helper()
	objs := make([]runtime.Object, 0, len(scs))
	for _, sc := range scs {
		objs = append(objs, sc)
	}
	clientset := fake.NewSimpleClientset(objs...)
	return NewMutator(
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		testVMIOperator(t, clientset),
	)
}

func newSC(name string, isDefault bool, provisioner, dataEngine string) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: name, Annotations: map[string]string{}},
		Provisioner: provisioner,
		Parameters:  map[string]string{},
	}
	if isDefault {
		sc.Annotations[util.AnnotationIsDefaultStorageClassName] = "true"
	}
	if dataEngine != "" {
		sc.Parameters[util.ParamDataEngine] = dataEngine
	}
	return sc
}

func cdiDefaultSC() *storagev1.StorageClass {
	return newSC("lh2", true, longhornProvisioner, "v2")
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

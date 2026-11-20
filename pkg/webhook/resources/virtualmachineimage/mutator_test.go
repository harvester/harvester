package virtualmachineimage

import (
	"testing"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestVirtualMachineImageMutatorDefaultsBackendFromControlPlaneLabel(t *testing.T) {
	tests := []struct {
		name  string
		nodes []*corev1.Node
		want  harvesterv1.VMIBackend
	}{
		{
			name:  "uses CDI when a control-plane node skipped the Longhorn default disk",
			nodes: []*corev1.Node{newNode("cp1", true, "false")},
			want:  harvesterv1.VMIBackendCDI,
		},
		{
			name:  "uses BackingImage when the control-plane node created the Longhorn default disk",
			nodes: []*corev1.Node{newNode("cp1", true, "true")},
			want:  harvesterv1.VMIBackendBackingImage,
		},
		{
			name:  "uses BackingImage for upgraded clusters without the label",
			nodes: []*corev1.Node{newNode("cp1", true, "")},
			want:  harvesterv1.VMIBackendBackingImage,
		},
		{
			name: "counts nodes that only carry the master role label",
			nodes: []*corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "m1", Labels: map[string]string{
				util.KubeMasterNodeLabelKey:                 "true",
				longhorntypes.NodeCreateDefaultDiskLabelKey: "false",
			}}}},
			want: harvesterv1.VMIBackendCDI,
		},
		{
			name:  "ignores worker nodes that skipped the Longhorn default disk",
			nodes: []*corev1.Node{newNode("cp1", true, "true"), newNode("worker1", false, "false")},
			want:  harvesterv1.VMIBackendBackingImage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vmi := newVirtualMachineImage("")

			_, err := newTestMutator(tc.nodes...).Create(nil, vmi)

			require.NoError(t, err)
			assert.Equal(t, tc.want, vmi.Spec.Backend)
		})
	}
}

func TestVirtualMachineImageMutatorAddsBackendWhenItIsOmitted(t *testing.T) {
	patchOps, err := newTestMutator(newNode("cp1", true, "false")).Create(nil, newVirtualMachineImage(""))

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

			_, err := newTestMutator(newNode("cp1", true, "false")).Create(nil, vmi)

			require.NoError(t, err)
			assert.Equal(t, backend, vmi.Spec.Backend)
		})
	}
}

func TestVirtualMachineImageMutatorHandlesOmittedBackendOnUpdate(t *testing.T) {
	tests := []struct {
		name       string
		label      string
		oldBackend harvesterv1.VMIBackend
		want       harvesterv1.VMIBackend
	}{
		{
			name:       "preserves backingImage when cluster default is CDI",
			label:      "false",
			oldBackend: harvesterv1.VMIBackendBackingImage,
			want:       harvesterv1.VMIBackendBackingImage,
		},
		{
			name:       "preserves CDI when cluster default is backingImage",
			label:      "true",
			oldBackend: harvesterv1.VMIBackendCDI,
			want:       harvesterv1.VMIBackendCDI,
		},
		{
			name:  "uses cluster default when both backends are empty",
			label: "false",
			want:  harvesterv1.VMIBackendCDI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newVMI := newVirtualMachineImage("")

			_, err := newTestMutator(newNode("cp1", true, tc.label)).Update(nil, newVirtualMachineImage(tc.oldBackend), newVMI)

			require.NoError(t, err)
			assert.Equal(t, tc.want, newVMI.Spec.Backend)
		})
	}
}

func newTestMutator(nodes ...*corev1.Node) types.Mutator {
	objs := make([]runtime.Object, 0, 1+len(nodes))
	objs = append(objs, defaultStorageClass())
	for _, node := range nodes {
		objs = append(objs, node)
	}
	clientset := fake.NewSimpleClientset(objs...)
	return NewMutator(
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		fakeclients.NodeCache(clientset.CoreV1().Nodes),
	)
}

func newNode(name string, controlPlane bool, createDefaultDisk string) *corev1.Node {
	labels := map[string]string{}
	if controlPlane {
		labels[util.KubeControlPlaneNodeLabelKey] = "true"
	}
	if createDefaultDisk != "" {
		labels[longhorntypes.NodeCreateDefaultDiskLabelKey] = createDefaultDisk
	}
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
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

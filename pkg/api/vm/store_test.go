package vm

import (
	"testing"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type recordingStore struct {
	types.Store
	deletedIDs []string
}

func (s *recordingStore) Delete(_ *types.APIRequest, _ *types.APISchema, id string) (types.APIObject, error) {
	s.deletedIDs = append(s.deletedIDs, id)
	return types.APIObject{ID: id}, nil
}

func newTestVM(namespace, name string, claimNames ...string) *kubevirtv1.VirtualMachine {
	volumes := make([]kubevirtv1.Volume, 0, len(claimNames))
	for _, claimName := range claimNames {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: claimName,
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			},
		})
	}

	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: map[string]string{},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Volumes: volumes,
				},
			},
		},
	}
}

func newTestPVC(namespace, name string, annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
		},
	}
}

func TestVMStoreDelete(t *testing.T) {
	const (
		namespace = "default"
		vmName    = "vm1"
	)

	tests := []struct {
		name                string
		vm                  *kubevirtv1.VirtualMachine
		pvcs                []*corev1.PersistentVolumeClaim
		removedDisks        []string
		expectErr           bool
		expectDeleted       bool
		expectRemovedPVCs   string
		expectAnnotationSet bool
	}{
		{
			name:      "vm not found",
			expectErr: true,
		},
		{
			name: "no removed disks",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "",
		},
		{
			name: "removed disk is added to the annotation",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:        []string{"disk2"},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk2",
		},
		{
			name: "golden image PVC is skipped",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", map[string]string{util.AnnotationGoldenImage: "true"}),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:        []string{"disk1", "disk2"},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk2",
		},
		{
			name:         "missing PVC fails the request",
			vm:           newTestVM(namespace, vmName, "disk1"),
			removedDisks: []string{"disk1"},
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.vm != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.vm))
			}
			for _, pvc := range tc.pvcs {
				assert.NoError(t, clientset.Tracker().Add(pvc))
			}

			inner := &recordingStore{}
			store := &vmStore{
				Store:    inner,
				vms:      fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
				vmCache:  fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
				pvcs:     fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
				pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
			}

			req := &types.APIRequest{
				Name:      vmName,
				Namespace: namespace,
				Query:     map[string][]string{"removedDisks": tc.removedDisks},
			}
			_, err := store.Delete(req, nil, namespace+"/"+vmName)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Empty(t, inner.deletedIDs)
				return
			}
			assert.NoError(t, err)

			if tc.expectDeleted {
				assert.Equal(t, []string{namespace + "/" + vmName}, inner.deletedIDs)
			}

			vm, err := clientset.KubevirtV1().VirtualMachines(namespace).Get(t.Context(), vmName, metav1.GetOptions{})
			assert.NoError(t, err)
			value, found := vm.Annotations[util.RemovedPVCsAnnotationKey]
			assert.Equal(t, tc.expectAnnotationSet, found)
			assert.Equal(t, tc.expectRemovedPVCs, value)
		})
	}
}

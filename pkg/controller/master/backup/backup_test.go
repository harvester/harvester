package backup

import (
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func TestResolveVolumeSnapshotClass(t *testing.T) {
	const csiDriverName = "rbd.csi.ceph.com"

	handler := &Handler{
		storageClassCache: storageClassCache{
			"fast-ceph": &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fast-ceph",
					Annotations: map[string]string{
						util.AnnotationStorageProfileSnapshotClass: "fast-ceph-snapclass",
					},
				},
				Provisioner: csiDriverName,
			},
			"standard-ceph": &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "standard-ceph"},
				Provisioner: csiDriverName,
			},
		},
		snapshotClassCache: volumeSnapshotClassCache{
			"fast-ceph-snapclass": &snapshotv1.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{Name: "fast-ceph-snapclass"},
				Driver:     csiDriverName,
			},
			"default-ceph-snapclass": &snapshotv1.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{Name: "default-ceph-snapclass"},
				Driver:     csiDriverName,
			},
			"backup-ceph-snapclass": &snapshotv1.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{Name: "backup-ceph-snapclass"},
				Driver:     csiDriverName,
			},
		},
	}
	csiDriverConfig := map[string]settings.CSIDriverInfo{
		csiDriverName: {
			VolumeSnapshotClassName:       "default-ceph-snapclass",
			BackupVolumeSnapshotClassName: "backup-ceph-snapclass",
		},
	}

	tests := []struct {
		name         string
		backupType   harvesterv1.BackupType
		storageClass string
		expected     string
	}{
		{
			name:         "snapshot uses storage class override",
			backupType:   harvesterv1.Snapshot,
			storageClass: "fast-ceph",
			expected:     "fast-ceph-snapclass",
		},
		{
			name:         "snapshot falls back to csi driver config",
			backupType:   harvesterv1.Snapshot,
			storageClass: "standard-ceph",
			expected:     "default-ceph-snapclass",
		},
		{
			name:         "backup keeps using backup csi driver config",
			backupType:   harvesterv1.Backup,
			storageClass: "fast-ceph",
			expected:     "backup-ceph-snapclass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumeBackup := harvesterv1.VolumeBackup{
				CSIDriverName: csiDriverName,
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To(tt.storageClass),
					},
				},
			}

			volumeSnapshotClass, err := handler.resolveVolumeSnapshotClass(tt.backupType, volumeBackup, csiDriverConfig)
			require.NoError(t, err)
			require.Equal(t, tt.expected, volumeSnapshotClass.Name)
		})
	}
}

func TestResolveVolumeSnapshotClassWithStorageClassOverrideAndNoCSIDriverConfig(t *testing.T) {
	const csiDriverName = "rbd.csi.ceph.com"

	handler := &Handler{
		storageClassCache: storageClassCache{
			"fast-ceph": &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fast-ceph",
					Annotations: map[string]string{
						util.AnnotationStorageProfileSnapshotClass: "fast-ceph-snapclass",
					},
				},
				Provisioner: csiDriverName,
			},
		},
		snapshotClassCache: volumeSnapshotClassCache{
			"fast-ceph-snapclass": &snapshotv1.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{Name: "fast-ceph-snapclass"},
				Driver:     csiDriverName,
			},
		},
	}
	volumeBackup := harvesterv1.VolumeBackup{
		CSIDriverName: csiDriverName,
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To("fast-ceph"),
			},
		},
	}

	volumeSnapshotClass, err := handler.resolveVolumeSnapshotClass(harvesterv1.Snapshot, volumeBackup, map[string]settings.CSIDriverInfo{})
	require.NoError(t, err)
	require.Equal(t, "fast-ceph-snapclass", volumeSnapshotClass.Name)
}

type storageClassCache map[string]*storagev1.StorageClass

func (c storageClassCache) Get(name string) (*storagev1.StorageClass, error) {
	storageClass, ok := c[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "storageclasses"}, name)
	}
	return storageClass, nil
}

func (c storageClassCache) List(selector labels.Selector) ([]*storagev1.StorageClass, error) {
	storageClasses := make([]*storagev1.StorageClass, 0, len(c))
	for _, storageClass := range c {
		if selector == nil || selector.Matches(labels.Set(storageClass.Labels)) {
			storageClasses = append(storageClasses, storageClass)
		}
	}
	return storageClasses, nil
}

func (c storageClassCache) AddIndexer(_ string, _ generic.Indexer[*storagev1.StorageClass]) {
}

func (c storageClassCache) GetByIndex(_, _ string) ([]*storagev1.StorageClass, error) {
	return nil, nil
}

type volumeSnapshotClassCache map[string]*snapshotv1.VolumeSnapshotClass

func (c volumeSnapshotClassCache) Get(name string) (*snapshotv1.VolumeSnapshotClass, error) {
	volumeSnapshotClass, ok := c[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "volumesnapshotclasses"}, name)
	}
	return volumeSnapshotClass, nil
}

func (c volumeSnapshotClassCache) List(selector labels.Selector) ([]*snapshotv1.VolumeSnapshotClass, error) {
	volumeSnapshotClasses := make([]*snapshotv1.VolumeSnapshotClass, 0, len(c))
	for _, volumeSnapshotClass := range c {
		if selector == nil || selector.Matches(labels.Set(volumeSnapshotClass.Labels)) {
			volumeSnapshotClasses = append(volumeSnapshotClasses, volumeSnapshotClass)
		}
	}
	return volumeSnapshotClasses, nil
}

func (c volumeSnapshotClassCache) AddIndexer(_ string, _ generic.Indexer[*snapshotv1.VolumeSnapshotClass]) {
}

func (c volumeSnapshotClassCache) GetByIndex(_, _ string) ([]*snapshotv1.VolumeSnapshotClass, error) {
	return nil, nil
}

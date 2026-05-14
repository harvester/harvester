package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	harvesterutil "github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestValidateProvisionerAndConfigAllowsStorageClassSnapshotClassOverride(t *testing.T) {
	const csiDriverName = "rbd.csi.ceph.com"

	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fast-ceph",
			Annotations: map[string]string{
				harvesterutil.AnnotationStorageProfileSnapshotClass: "fast-ceph-snapclass",
			},
		},
		Provisioner: csiDriverName,
	})
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-disk",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("fast-ceph"),
		},
	}

	err := ValidateProvisionerAndConfig(
		pvc,
		nil,
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		v1beta1.Snapshot,
		map[string]settings.CSIDriverInfo{},
	)
	require.NoError(t, err)
}

func TestValidateProvisionerAndConfigRequiresCSIDriverConfigWithoutStorageClassSnapshotClassOverride(t *testing.T) {
	const csiDriverName = "rbd.csi.ceph.com"

	clientset := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "standard-ceph"},
		Provisioner: csiDriverName,
	})
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-disk",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("standard-ceph"),
		},
	}

	err := ValidateProvisionerAndConfig(
		pvc,
		nil,
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		v1beta1.Snapshot,
		map[string]settings.CSIDriverInfo{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not configured")
}

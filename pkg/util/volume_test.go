package util

import (
	"testing"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestGetPVCStorageClassName(t *testing.T) {
	const (
		defaultStorageClass  = "default-sc"
		explicitStorageClass = "explicit-sc"
	)

	scCache := testStorageClassCache{
		defaultStorageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultStorageClass,
				Annotations: map[string]string{
					AnnotationIsDefaultStorageClassName: "true",
				},
			},
		},
		explicitStorageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{Name: explicitStorageClass},
		},
	}

	require.Equal(t, explicitStorageClass, GetPVCStorageClassName(&corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To(explicitStorageClass)},
	}, scCache))
	require.Equal(t, defaultStorageClass, GetPVCStorageClassName(&corev1.PersistentVolumeClaim{}, scCache))
}

func TestGetPVCStorageClassSnapshotClassNameUsesDefaultStorageClass(t *testing.T) {
	const (
		defaultStorageClass = "default-sc"
		snapshotClass       = "default-snapshot-class"
	)

	scCache := testStorageClassCache{
		defaultStorageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultStorageClass,
				Annotations: map[string]string{
					AnnotationIsDefaultStorageClassName:   "true",
					AnnotationStorageProfileSnapshotClass: snapshotClass,
				},
			},
		},
	}

	require.Equal(t, snapshotClass, GetPVCStorageClassSnapshotClassName(&corev1.PersistentVolumeClaim{}, scCache))
}

type testStorageClassCache map[string]*storagev1.StorageClass

func (c testStorageClassCache) Get(name string) (*storagev1.StorageClass, error) {
	storageClass, ok := c[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "storageclasses"}, name)
	}
	return storageClass, nil
}

func (c testStorageClassCache) List(selector labels.Selector) ([]*storagev1.StorageClass, error) {
	storageClasses := make([]*storagev1.StorageClass, 0, len(c))
	for _, storageClass := range c {
		if selector == nil || selector.Matches(labels.Set(storageClass.Labels)) {
			storageClasses = append(storageClasses, storageClass)
		}
	}
	return storageClasses, nil
}

func (c testStorageClassCache) AddIndexer(_ string, _ generic.Indexer[*storagev1.StorageClass]) {
}

func (c testStorageClassCache) GetByIndex(_, _ string) ([]*storagev1.StorageClass, error) {
	return nil, nil
}

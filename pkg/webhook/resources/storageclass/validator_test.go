package storageclass

import (
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_storageClassValidator_Delete(t *testing.T) {
	tests := []struct {
		name         string
		storageClass *storagev1.StorageClass
		expectError  bool
	}{
		{
			name: "storage class any with AnnotationIsReservedStorageClass true can't be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "any",
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "true",
					},
				},
			},
			expectError: true,
		},
		{
			name: "storage class any with AnnotationIsReservedStorageClass false can be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "any",
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "false",
					},
				},
			},
			expectError: false,
		},
		{
			name: "storage class harvester-longhorn with AnnotationIsReservedStorageClass false can be deleted too",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: util.StorageClassHarvesterLonghorn,
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "false",
					},
				},
			},
			expectError: false,
		},
		{
			name: "storage class harvester-longhorn without AnnotationIsReservedStorageClass can't be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: util.StorageClassHarvesterLonghorn,
					Annotations: map[string]string{
						util.HelmReleaseNameAnnotation:      util.HarvesterChartReleaseName,
						util.HelmReleaseNamespaceAnnotation: util.HarvesterSystemNamespaceName,
					},
				},
			},
			expectError: true,
		},
		{
			name: "storage class others without AnnotationIsReservedStorageClass can be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "others",
				},
			},
			expectError: false,
		},
	}

	fakeStorageClassCache := fakeclients.StorageClassCache(corefake.NewSimpleClientset().StorageV1().StorageClasses)
	validator := NewValidator(fakeStorageClassCache).(*storageClassValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Delete(nil, tc.storageClass)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

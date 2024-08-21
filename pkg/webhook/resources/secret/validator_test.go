package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	storageClasses = []*storagev1.StorageClass{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc1",
			},
			Parameters: map[string]string{
				util.LonghornOptionEncrypted:          "true",
				util.CSIProvisionerSecretNameKey:      "test-secret",
				util.CSIProvisionerSecretNamespaceKey: "default",
				util.CSINodeStageSecretNameKey:        "test-secret",
				util.CSINodeStageSecretNamespaceKey:   "default",
				util.CSINodePublishSecretNameKey:      "test-secret",
				util.CSINodePublishSecretNamespaceKey: "default",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc2",
			},
			Parameters: map[string]string{},
		},
	}
)

func Test_secretValidator_Delete(t *testing.T) {
	tests := []struct {
		name        string
		secret      *corev1.Secret
		expectError bool
	}{
		{
			name: "secret used by encrypted storage class",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
			},
			expectError: true,
		},
		{
			name: "secret not used by encrypted storage class",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-not-used",
					Namespace: "default",
				},
			},
			expectError: false,
		},
	}

	coreclientset := corefake.NewSimpleClientset()
	for _, sc := range storageClasses {
		err := coreclientset.Tracker().Add(sc.DeepCopy())
		assert.Nil(t, err, "Mock resource should add into fake controller tracker")
	}

	fakeStorageClassCache := fakeclients.StorageClassCache(coreclientset.StorageV1().StorageClasses)
	validator := NewValidator(fakeStorageClassCache).(*secretValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Delete(nil, tc.secret)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

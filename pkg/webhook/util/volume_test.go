package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	harvesterutil "github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckLonghornV2EncryptedExpand(t *testing.T) {
	tests := []struct {
		name          string
		pvc           *corev1.PersistentVolumeClaim
		sc            *storagev1.StorageClass
		expectError   bool
		errorContains string
	}{
		{
			name:          "blocks Longhorn V2 encrypted PVC",
			pvc:           newTestPVC("test-pvc", "longhorn-v2-encrypted"),
			sc:            newTestStorageClass("longhorn-v2-encrypted", harvesterutil.CSIProvisionerLonghorn, map[string]string{harvesterutil.ParamDataEngine: "v2", harvesterutil.LonghornOptionEncrypted: "true"}),
			expectError:   true,
			errorContains: "expansion of Longhorn V2 encrypted PVC default/test-pvc is temporarily not supported",
		},
		{
			name: "allows Longhorn V2 unencrypted PVC",
			pvc:  newTestPVC("test-pvc", "longhorn-v2"),
			sc:   newTestStorageClass("longhorn-v2", harvesterutil.CSIProvisionerLonghorn, map[string]string{harvesterutil.ParamDataEngine: "v2", harvesterutil.LonghornOptionEncrypted: "false"}),
		},
		{
			name: "allows Longhorn V1 encrypted PVC",
			pvc:  newTestPVC("test-pvc", "longhorn-v1-encrypted"),
			sc:   newTestStorageClass("longhorn-v1-encrypted", harvesterutil.CSIProvisionerLonghorn, map[string]string{harvesterutil.ParamDataEngine: "v1", harvesterutil.LonghornOptionEncrypted: "true"}),
		},
		{
			name: "allows non-Longhorn PVC",
			pvc:  newTestPVC("test-pvc", "standard"),
			sc:   newTestStorageClass("standard", "rancher.io/local-path", map[string]string{harvesterutil.ParamDataEngine: "v2", harvesterutil.LonghornOptionEncrypted: "true"}),
		},
		{
			name:          "blocks PVC without storage class when default is Longhorn V2 encrypted",
			pvc:           newTestPVCWithoutStorageClass("test-pvc"),
			sc:            newDefaultTestStorageClass("longhorn-v2-encrypted", harvesterutil.CSIProvisionerLonghorn, map[string]string{harvesterutil.ParamDataEngine: "v2", harvesterutil.LonghornOptionEncrypted: "true"}),
			expectError:   true,
			errorContains: "expansion of Longhorn V2 encrypted PVC default/test-pvc is temporarily not supported",
		},
		{
			name: "allows PVC with explicit empty storage class",
			pvc:  newTestPVCWithEmptyStorageClass("test-pvc"),
			sc:   newDefaultTestStorageClass("longhorn-v2-encrypted", harvesterutil.CSIProvisionerLonghorn, map[string]string{harvesterutil.ParamDataEngine: "v2", harvesterutil.LonghornOptionEncrypted: "true"}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.sc != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.sc))
			}

			err := checkLonghornV2EncryptedExpand(
				tc.pvc,
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
			)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}

			err = CheckExpand(
				tc.pvc,
				fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
				nil,
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				nil,
			)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func newTestPVC(name, storageClassName string) *corev1.PersistentVolumeClaim {
	return newTestPVCWithStorageClassName(name, &storageClassName)
}

func newTestPVCWithoutStorageClass(name string) *corev1.PersistentVolumeClaim {
	return newTestPVCWithStorageClassName(name, nil)
}

func newTestPVCWithEmptyStorageClass(name string) *corev1.PersistentVolumeClaim {
	storageClassName := ""
	return newTestPVCWithStorageClassName(name, &storageClassName)
}

func newTestPVCWithStorageClassName(name string, storageClassName *string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	pvc.Spec.StorageClassName = storageClassName
	return pvc
}

func newTestStorageClass(name, provisioner string, parameters map[string]string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: provisioner,
		Parameters:  parameters,
	}
}

func newDefaultTestStorageClass(name, provisioner string, parameters map[string]string) *storagev1.StorageClass {
	sc := newTestStorageClass(name, provisioner, parameters)
	sc.Annotations = map[string]string{
		harvesterutil.AnnotationIsDefaultStorageClassName: "true",
	}
	return sc
}

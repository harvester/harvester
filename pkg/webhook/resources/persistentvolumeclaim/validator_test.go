package persistentvolumeclaim

import (
	"testing"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newUpgradeImage(namespace, name string) *harvesterv1.VirtualMachineImage {
	return &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				util.AnnotationUpgradeImage: "True",
			},
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			TargetStorageClassName: util.StorageClassLonghornStatic,
		},
	}
}

func TestIsBelongToUpgradeImage(t *testing.T) {
	tests := []struct {
		name           string
		pvc            *corev1.PersistentVolumeClaim
		dataPVC        *corev1.PersistentVolumeClaim
		image          *harvesterv1.VirtualMachineImage
		expectedResult bool
		expectError    bool
	}{
		{
			name: "PVC owned by DataVolume with upgrade image annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cdi.kubevirt.io/v1beta1",
							Kind:       util.DVObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "PVC owned by PVC with upgrade image annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cdi.kubevirt.io/v1beta1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "PVC owned by DataVolume with upgrade image annotation outside system namespace",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cdi.kubevirt.io/v1beta1",
							Kind:       util.DVObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage("default", "upgrade-image"),
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "scratch PVC owned by importer pod for upgrade image PVC",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test-scratch",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "scratch PVC owned by importer pod but data PVC is not an upgrade image PVC",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test-scratch",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: util.HarvesterSystemNamespaceName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "scratch PVC owned by importer pod but name does not match data PVC scratch name",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-scratch",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "scratch PVC owned by importer pod but pod owner is not controller",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test-scratch",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:          newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "PVC owned by DataVolume without upgrade annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cdi.kubevirt.io/v1beta1",
							Kind:       util.DVObjectName,
							Name:       "normal-image",
						},
					},
				},
			},
			image: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "normal-image",
					Namespace: util.HarvesterSystemNamespaceName,
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "PVC with longhorn-static sc owned by DataVolume but image not found",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "cdi.kubevirt.io/v1beta1",
							Kind:       util.DVObjectName,
							Name:       "non-existent-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "PVC with longhorn-static sc with no owner references",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "PVC with no owner references",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
			},
			expectedResult: false,
			expectError:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			if tc.image != nil {
				err := clientset.Tracker().Add(tc.image)
				assert.Nil(t, err, "Failed to add image to fake client")
			}
			if tc.dataPVC != nil {
				err := clientset.Tracker().Add(tc.dataPVC)
				assert.Nil(t, err, "Failed to add data PVC to fake client")
			}

			validator := &pvcValidator{
				imageCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				pvcCache:   fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
			}

			result, err := validator.isBelongToUpgradeImage(tc.pvc)

			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
				assert.Equal(t, tc.expectedResult, result, tc.name)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	const (
		biName  = "vmi-test-bi"
		imageID = "default/image-szq79"
	)

	newLonghornSC := func(name, backingImage string) *storagev1.StorageClass {
		return &storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: name},
			Provisioner: util.CSIProvisionerLonghorn,
			Parameters:  map[string]string{util.LonghornOptionBackingImageName: backingImage},
		}
	}
	newBI := func(annotationImageID string) *longhorn.BackingImage {
		annotations := map[string]string{}
		if annotationImageID != "" {
			annotations[util.AnnotationImageID] = annotationImageID
		}
		return &longhorn.BackingImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:        biName,
				Namespace:   util.LonghornSystemNamespaceName,
				Annotations: annotations,
			},
		}
	}

	tests := []struct {
		name          string
		pvc           *corev1.PersistentVolumeClaim
		dataPVC       *corev1.PersistentVolumeClaim
		image         *harvesterv1.VirtualMachineImage
		sc            *storagev1.StorageClass
		bi            *longhorn.BackingImage
		sarDenied     bool
		expectError   bool
		errorContains string
	}{
		{
			name: "create PVC with regular storage class",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassHarvesterLonghorn),
				},
			},
			sc:          newLonghornSC(util.StorageClassHarvesterLonghorn, ""),
			expectError: false,
		},
		{
			name: "create PVC without storage class",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{},
			},
			expectError: false,
		},
		{
			name: "create PVC with reserved longhorn-static storage class",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			expectError:   true,
			errorContains: "reserved storage class",
		},
		{
			name: "create scratch PVC with reserved longhorn-static storage class for upgrade image import",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test-scratch",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: util.HarvesterSystemNamespaceName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:       newUpgradeImage(util.HarvesterSystemNamespaceName, "upgrade-image"),
			expectError: false,
		},
		{
			name: "reject scratch PVC with reserved longhorn-static storage class outside system namespace",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test-scratch",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Pod",
							Name:       "importer-prime-test",
							UID:        "pod-uid",
							Controller: ptr.To(true),
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			dataPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prime-test",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       util.PVCObjectName,
							Name:       "upgrade-image",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassLonghornStatic),
				},
			},
			image:         newUpgradeImage("default", "upgrade-image"),
			expectError:   true,
			errorContains: "reserved storage class",
		},
		{
			name: "create PVC with reserved vmstate-persistence storage class",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassVmstatePersistence),
				},
			},
			expectError:   true,
			errorContains: "reserved storage class",
		},
		{
			name: "create PVC with reserved vmstate-persistence storage class managed by KubeVirt",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persistent-state-for-vm1",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelKubeVirtPersistentState: "vm1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubevirt.io/v1",
							Kind:       "VirtualMachine",
							Name:       "vm1",
							UID:        "test-uid",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassVmstatePersistence),
				},
			},
			expectError: false,
		},
		{
			name: "create PVC with reserved vmstate-persistence storage class with label but no owner reference",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persistent-state-for-vm1",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelKubeVirtPersistentState: "vm1",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassVmstatePersistence),
				},
			},
			expectError:   true,
			errorContains: "reserved storage class",
		},
		{
			name: "create PVC with reserved vmstate-persistence storage class with mismatched label and owner",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persistent-state-for-vm1",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelKubeVirtPersistentState: "vm1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubevirt.io/v1",
							Kind:       "VirtualMachine",
							Name:       "vm2",
							UID:        "test-uid",
						},
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(util.StorageClassVmstatePersistence),
				},
			},
			expectError:   true,
			errorContains: "reserved storage class",
		},
		{
			name: "create PVC with Longhorn SC that has backingImage, SAR allowed",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("lh-test-sc"),
				},
			},
			sc:          newLonghornSC("lh-test-sc", biName),
			bi:          newBI(imageID),
			expectError: false,
		},
		{
			name: "create PVC with Longhorn SC that has backingImage, SAR denied",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("lh-test-sc"),
				},
			},
			sc:          newLonghornSC("lh-test-sc", biName),
			bi:          newBI(imageID),
			sarDenied:   true,
			expectError: true,
		},
	}

	allowedFakeSAR := fakeclients.AllowedSARClient()
	denyFakeSAR := fakeclients.DeniedSARClient()

	fakeRequest := fakeclients.NewFakeRequest("test-user")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.sc != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.sc))
			}
			if tc.bi != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.bi))
			}
			if tc.image != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.image))
			}
			if tc.dataPVC != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.dataPVC))
			}

			var sar = allowedFakeSAR
			if tc.sarDenied {
				sar = denyFakeSAR
			}
			validator := &pvcValidator{
				pvcCache:          fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				imageCache:        fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				scCache:           fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				backingImageCache: fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages),
				sar:               sar,
			}

			err := validator.Create(fakeRequest, tc.pvc)

			if tc.expectError {
				assert.NotNil(t, err, tc.name)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, tc.name)
				}
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

func Test_PVCDeletion(t *testing.T) {
	deletingVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-vm",
			Namespace:         "default",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
	}

	nonDeletingVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-deleting-vm",
			Namespace: "default",
		},
	}

	for _, tc := range []struct {
		name        string
		pvc         *corev1.PersistentVolumeClaim
		expectError bool
	}{
		{
			name: "PVC owned by deleting VM",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubevirt.io/v1",
							Kind:       "VirtualMachine",
							Name:       deletingVM.Name,
							UID:        "test-uid",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "PVC owned by non-deleting VM",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "kubevirt.io/v1",
							Kind:       "VirtualMachine",
							Name:       nonDeletingVM.Name,
							UID:        "test-uid",
						},
					},
				},
			},
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(deletingVM, nonDeletingVM, tc.pvc)

			validator := &pvcValidator{
				vmCache:    fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
				pvcCache:   fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				imageCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
			}

			err := validator.validateOwnerReferences(tc.pvc)

			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}

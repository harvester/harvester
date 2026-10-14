package cdi

import (
	"context"
	"net/http"
	"testing"

	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func cloneTestBackend(t *testing.T, clientset *fake.Clientset) *Backend {
	t.Helper()
	vmio, err := common.GetVMIOperator(
		fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses), http.Client{},
	)
	require.NoError(t, err)
	return GetBackend(context.Background(),
		&namespacedDataVolumes{clientset: clientset},
		fakeclients.StorageClassClient(clientset.StorageV1().StorageClasses),
		fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		vmio, nil, nil).(*Backend)
}

func TestInitializeImageCryptoClone(t *testing.T) {
	// Verify clone initialization for both supported crypto conversions.
	for _, tt := range []struct {
		name      string
		operation harvesterv1.VirtualMachineImageCryptoOperationType
	}{
		{
			name:      "initialize encrypted image from unencrypted source",
			operation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
		},
		{
			name:      "initialize decrypted image from encrypted source",
			operation: harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns", UID: "source-uid"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status:     harvesterv1.VirtualMachineImageStatus{StorageClassName: "source-sc", Size: 1 << 29, VirtualSize: (1 << 30) - 10},
			}
			harvesterv1.ImageImported.True(source)
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "target-ns", UID: "target-uid"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: harvesterv1.VMIBackendCDI, SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
					TargetStorageClassName: "target-sc",
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						SourceImageName: source.Name, SourceImageNamespace: source.Namespace, CryptoOperation: tt.operation,
					},
				},
			}

			// Configure Longhorn V2 StorageClasses on opposite sides of the
			// encryption boundary for the selected operation.
			targetSC := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{Name: target.Spec.TargetStorageClassName}, Provisioner: util.CSIProvisionerLonghorn,
				Parameters: map[string]string{util.ParamDataEngine: "v2", util.LonghornOptionEncrypted: "true"},
			}
			sourceSC := targetSC.DeepCopy()
			sourceSC.Name = source.Status.StorageClassName
			sourceSC.Parameters[util.LonghornOptionEncrypted] = "false"
			if tt.operation == harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt {
				targetSC.Parameters[util.LonghornOptionEncrypted] = "false"
				sourceSC.Parameters[util.LonghornOptionEncrypted] = "true"
			}
			sourcePVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: source.Name, Namespace: source.Namespace},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new(sourceSC.Name),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: *resource.NewQuantity(source.Status.VirtualSize, resource.BinarySI)},
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase:    corev1.ClaimBound,
					Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			}

			// Build the CDI backend with all resources stored in fake clients.
			clientset := fake.NewSimpleClientset(source, target, sourceSC, targetSC, sourcePVC)
			b := cloneTestBackend(t, clientset)

			// Initialize the clone. This sets both target image size fields and
			// creates the DataVolume, but must not mark the image as imported yet.
			updated, err := b.Initialize(target)
			require.NoError(t, err)
			assert.Equal(t, sourcePVC.Status.Capacity.Storage().Value(), updated.Status.Size)
			assert.Equal(t, source.Status.VirtualSize, updated.Status.VirtualSize)
			assert.False(t, harvesterv1.ImageImported.IsTrue(updated), "initializing sizes must not mark the image imported")
			initialImageUpdates := 0
			for _, action := range clientset.Actions() {
				if action.GetResource().Resource == "virtualmachineimages" && action.GetVerb() == "update" {
					initialImageUpdates++
				}
				if action.GetResource().Resource == "persistentvolumeclaims" {
					assert.Equal(t, "get", action.GetVerb(), "initialization must use the existing source PVC")
				}
			}
			assert.Equal(t, 1, initialImageUpdates, "both image size fields must be initialized together")

			// Verify that the DataVolume requests a host-assisted copy from the
			// source PVC and preserves the image's virtual size.
			dv, err := b.dataVolumeClient.Get(target.Namespace, target.Name, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, "false", dv.Annotations[cdicommon.AnnUsePopulator], "crypto conversions must use host-assisted copying")
			assert.Equal(t, "target-sc", *dv.Spec.Storage.StorageClassName)
			assert.Zero(t, dv.Spec.Storage.Resources.Requests.Storage().Cmp(resource.MustParse("1Gi")))
			assert.Nil(t, dv.Spec.Storage.VolumeMode, "CDI must select the volume mode from the target StorageProfile")
			assert.Empty(t, dv.Spec.Storage.AccessModes, "CDI must select the access modes from the target StorageProfile")
			require.Len(t, dv.OwnerReferences, 1)
			assert.Equal(t, metav1.OwnerReference{
				APIVersion:         common.HarvesterAPIV1Beta1,
				Kind:               common.VMImageKind,
				Name:               target.Name,
				UID:                target.UID,
				BlockOwnerDeletion: new(true),
			}, dv.OwnerReferences[0], "ownership must be preserved when the image has no TypeMeta")
			assert.Equal(t, source.Name, dv.Spec.Source.PVC.Name)
			assert.Equal(t, source.Namespace, dv.Spec.Source.PVC.Namespace)

			// Reinitializing an existing clone must be safe and idempotent.
			_, err = b.Initialize(updated)
			require.NoError(t, err, "initialization must be idempotent")
		})
	}
}

func TestCloneRequiresValidSourcePVC(t *testing.T) {
	for _, tt := range []struct {
		name          string
		pvc           *corev1.PersistentVolumeClaim
		expectedError string
	}{
		{
			name:          "reject clone when source PVC does not exist",
			pvc:           nil,
			expectedError: `failed to get source image PVC: persistentvolumeclaims "source" not found`,
		},
		{
			name: "reject clone when source PVC is pending",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
			},
			expectedError: "source image PVC source-ns/source is not ready",
		},
		{
			name: "reject clone when bound source PVC is being deleted",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source", Namespace: "source-ns", DeletionTimestamp: new(metav1.Now()),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase:    corev1.ClaimBound,
					Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
			expectedError: "source image PVC source-ns/source is not ready",
		},
		{
			name: "reject clone when bound source PVC has no storage request or capacity",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"},
				Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
			},
			expectedError: "source image PVC has no storage capacity",
		},
		{
			name: "initialize clone when source PVC has a positive request and no reported capacity",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
			},
		},
		{
			name: "initialize clone when source PVC has positive capacity and no storage request",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase:    corev1.ClaimBound,
					Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns", UID: "source-uid"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status:     harvesterv1.VirtualMachineImageStatus{StorageClassName: "source-sc", Size: 1 << 29, VirtualSize: 1 << 30},
			}
			harvesterv1.ImageImported.True(source)
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "target-ns", UID: "target-uid"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: harvesterv1.VMIBackendCDI, SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
					TargetStorageClassName: "target-sc",
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						SourceImageName: source.Name, SourceImageNamespace: source.Namespace,
						CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
					},
				},
			}
			clientset := fake.NewSimpleClientset(source, target)
			if tt.pvc != nil {
				require.NoError(t, clientset.Tracker().Add(tt.pvc))
			}
			if tt.expectedError == "" {
				require.NoError(t, clientset.Tracker().Add(&storagev1.StorageClass{
					ObjectMeta:  metav1.ObjectMeta{Name: target.Spec.TargetStorageClassName},
					Provisioner: util.CSIProvisionerLonghorn,
					Parameters:  map[string]string{util.ParamDataEngine: "v2", util.LonghornOptionEncrypted: "true"},
				}))
			}
			b := cloneTestBackend(t, clientset)
			updated, err := b.Initialize(target)
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				for _, action := range clientset.Actions() {
					assert.Equal(t, "get", action.GetVerb(), "an invalid source PVC must not cause resource creation or updates")
				}
				_, err = b.dataVolumeClient.Get(target.Namespace, target.Name, metav1.GetOptions{})
				assert.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, source.Status.VirtualSize, updated.Status.Size)
			assert.Equal(t, source.Status.VirtualSize, updated.Status.VirtualSize)
			assert.False(t, harvesterv1.ImageImported.IsTrue(updated))
			dv, err := b.dataVolumeClient.Get(target.Namespace, target.Name, metav1.GetOptions{})
			require.NoError(t, err)
			assert.Zero(t, dv.Spec.Storage.Resources.Requests.Storage().Cmp(resource.MustParse("1Gi")))
			assert.Equal(t, source.Name, dv.Spec.Source.PVC.Name)
			assert.Equal(t, source.Namespace, dv.Spec.Source.PVC.Namespace)
		})
	}
}

func TestCloneRequiresValidSourceImage(t *testing.T) {
	for _, tt := range []struct {
		name          string
		source        *harvesterv1.VirtualMachineImage
		expectedError string
	}{
		{
			name:          "reject clone when source image does not exist",
			source:        nil,
			expectedError: `failed to get source image: virtualmachineimages.harvesterhci.io "source" not found`,
		},
		{
			name: "reject clone when source image virtual size is zero",
			source: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns", UID: "source-uid"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status: harvesterv1.VirtualMachineImageStatus{
					StorageClassName: "source-sc", Size: 1 << 29,
					Conditions: []harvesterv1.Condition{{Type: harvesterv1.ImageImported, Status: corev1.ConditionTrue}},
				},
			},
			expectedError: "source image source-ns/source has no virtual size",
		},
		{
			name: "reject clone when source image virtual size is negative",
			source: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns", UID: "source-uid"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status: harvesterv1.VirtualMachineImageStatus{
					StorageClassName: "source-sc", Size: 1 << 29, VirtualSize: -1,
					Conditions: []harvesterv1.Condition{{Type: harvesterv1.ImageImported, Status: corev1.ConditionTrue}},
				},
			},
			expectedError: "source image source-ns/source has no virtual size",
		},
		{
			name: "reject clone when imported source image is being deleted",
			source: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source", Namespace: "source-ns", UID: "source-uid", DeletionTimestamp: new(metav1.Now()),
				},
				Spec: harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status: harvesterv1.VirtualMachineImageStatus{
					StorageClassName: "source-sc", Size: 1 << 29, VirtualSize: 1 << 30,
					Conditions: []harvesterv1.Condition{{Type: harvesterv1.ImageImported, Status: corev1.ConditionTrue}},
				},
			},
			expectedError: "source image source-ns/source is not ready",
		},
		{
			name: "reject clone when source image is not imported",
			source: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns", UID: "source-uid"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI},
				Status: harvesterv1.VirtualMachineImageStatus{
					StorageClassName: "source-sc", Size: 1 << 29, VirtualSize: 1 << 30,
					Conditions: []harvesterv1.Condition{{Type: harvesterv1.ImageImported, Status: corev1.ConditionFalse}},
				},
			},
			expectedError: "source image source-ns/source is not ready",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "target-ns", UID: "target-uid"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: harvesterv1.VMIBackendCDI, SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
					TargetStorageClassName: "target-sc",
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						SourceImageName: "source", SourceImageNamespace: "source-ns",
						CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
					},
				},
			}
			clientset := fake.NewSimpleClientset(target)
			if tt.source != nil {
				require.NoError(t, clientset.Tracker().Add(tt.source))
			}
			b := cloneTestBackend(t, clientset)
			_, err := b.Initialize(target)
			require.EqualError(t, err, tt.expectedError)
			for _, action := range clientset.Actions() {
				assert.Equal(t, "get", action.GetVerb(), "an invalid source image must not mutate resources")
			}
			_, err = b.dataVolumeClient.Get(target.Namespace, target.Name, metav1.GetOptions{})
			assert.True(t, apierrors.IsNotFound(err))
		})
	}
}

// The repository's versioned CDI fake is cluster-scoped. Use the tracker with
// an explicit namespace to model the real DataVolume API.
var dataVolumeResource = schema.GroupVersionResource{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datavolumes"}

type namespacedDataVolumes struct {
	ctlcdiv1.DataVolumeClient
	clientset *fake.Clientset
}

func (c *namespacedDataVolumes) Create(dv *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	return dv, c.clientset.Tracker().Create(dataVolumeResource, dv, dv.Namespace)
}
func (c *namespacedDataVolumes) Get(namespace, name string, _ metav1.GetOptions) (*cdiv1.DataVolume, error) {
	obj, err := c.clientset.Tracker().Get(dataVolumeResource, namespace, name)
	if err != nil {
		return nil, err
	}
	return obj.(*cdiv1.DataVolume), nil
}

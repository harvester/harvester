package virtualmachineimage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	cditype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestVirtualMachineImageValidator_Create(t *testing.T) {
	tests := []struct {
		name        string
		vmi         *harvesterv1.VirtualMachineImage
		sarDenied   bool
		expectError bool
	}{
		{
			name: "download image with valid URL - pass",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-img", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
					URL:         "http://example.com/image.iso",
				},
			},
			expectError: false,
		},
		{
			name: "clone image, SAR denied for source image - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-clone", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-clone-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			sarDenied:   true,
			expectError: true,
		},
		{
			// SAR passes but source image doesn't exist — confirms the check proceeds past SAR
			name: "clone image, SAR allowed, source not found - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-clone-2", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-clone-image-2",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			sarDenied:   false,
			expectError: true,
		},
	}

	allowedFakeSAR := fakeclients.AllowedSARClient()
	denyFakeSAR := fakeclients.DeniedSARClient()

	fakeRequest := fakeclients.NewFakeRequest("test-user")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			sar := allowedFakeSAR
			if tc.sarDenied {
				sar = denyFakeSAR
			}

			validator := NewValidator(
				fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.PodCache(clientset.CoreV1().Pods),
				nil, nil,
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				nil, nil, nil,
				sar,
			)

			err := validator.Create(fakeRequest, tc.vmi)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteCDIImage(t *testing.T) {
	image := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{Name: "image", Namespace: "default"},
		Spec: harvesterv1.VirtualMachineImageSpec{
			Backend:                harvesterv1.VMIBackendCDI,
			SourceType:             harvesterv1.VirtualMachineImageSourceTypeExportVolume,
			PVCName:                "export-source",
			PVCNamespace:           "default",
			TargetStorageClassName: "shared-sc",
		},
		Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: "shared-sc"},
	}

	stoppedVM := imageVM("default", "image")
	runningVM := stoppedVM.DeepCopy()
	runningVM.Status.Ready = true
	runStrategy := kubevirtv1.RunStrategyAlways
	runningVM.Spec.RunStrategy = &runStrategy
	deletingVM := stoppedVM.DeepCopy()
	now := metav1.Now()
	deletingVM.DeletionTimestamp = &now
	cloneVM := imageVM("default", "clone")
	cloneVM.Annotations = map[string]string{
		util.AnnotationVolumeClaimTemplates: `[{"metadata":{"name":"clone","annotations":` +
			`{"harvesterhci.io/imageId":"default/image"}}}]`,
	}
	cloneDV := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "clone", Namespace: "default"},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{PVC: &cdiv1.DataVolumeSourcePVC{
				Namespace: "default", Name: "image",
			}},
		},
		Status: cdiv1.DataVolumeStatus{Phase: cdiv1.Succeeded},
	}
	migratingVM := imageVM("default", "target")
	migratingVM.Annotations = map[string]string{
		util.AnnotationVolumeClaimTemplates: `[{"metadata":{"name":"image"},"targetVolume":"target"}]`,
	}

	conflict := "cannot delete image default/image: its PVC is used by VM default/imported"
	tests := []struct {
		name     string
		vms      []*kubevirtv1.VirtualMachine
		pvcs     []*corev1.PersistentVolumeClaim
		dvs      []*cdiv1.DataVolume
		emptySC  bool
		cacheErr error
		wantErr  string
		reason   metav1.StatusReason
	}{
		{name: "image PVC alone does not block deletion"},
		{
			name:    "stopped VM uses the image PVC",
			vms:     []*kubevirtv1.VirtualMachine{stoppedVM},
			wantErr: conflict,
			reason:  metav1.StatusReasonConflict,
		},
		{
			name:    "running VM uses the image PVC",
			vms:     []*kubevirtv1.VirtualMachine{runningVM},
			wantErr: conflict,
			reason:  metav1.StatusReasonConflict,
		},
		{
			name:    "direct use before storage class status is set",
			vms:     []*kubevirtv1.VirtualMachine{stoppedVM},
			emptySC: true,
			wantErr: conflict,
			reason:  metav1.StatusReasonConflict,
		},
		{
			name: "same PVC name in another namespace",
			vms:  []*kubevirtv1.VirtualMachine{imageVM("other", "image")},
		},
		{
			name:    "VM uses a separate completed clone of the image",
			vms:     []*kubevirtv1.VirtualMachine{cloneVM},
			pvcs:    []*corev1.PersistentVolumeClaim{imagePVC("default", "clone", "shared-sc")},
			dvs:     []*cdiv1.DataVolume{cloneDV},
			wantErr: "cannot delete image default/image: being used by clone PVC default/clone",
			reason:  metav1.StatusReasonConflict,
		},
		{
			name:    "completed clone PVC without a VM still blocks deletion",
			pvcs:    []*corev1.PersistentVolumeClaim{imagePVC("default", "clone", "shared-sc")},
			dvs:     []*cdiv1.DataVolume{cloneDV},
			wantErr: "cannot delete image default/image: being used by clone PVC default/clone",
			reason:  metav1.StatusReasonConflict,
		},
		{
			name: "clone PVC has been removed",
			dvs:  []*cdiv1.DataVolume{cloneDV},
		},
		{
			name: "unrelated PVC uses the shared storage class",
			pvcs: []*corev1.PersistentVolumeClaim{imagePVC("other", "unrelated", "shared-sc")},
		},
		{
			name: "VM uses the export source PVC",
			vms:  []*kubevirtv1.VirtualMachine{imageVM("default", "export-source")},
		},
		{
			name:    "source PVC remains in use during storage migration",
			vms:     []*kubevirtv1.VirtualMachine{migratingVM},
			wantErr: conflict,
			reason:  metav1.StatusReasonConflict,
		},
		{
			name: "deleting VM no longer blocks image cleanup",
			vms:  []*kubevirtv1.VirtualMachine{deletingVM},
		},
		{
			name:     "VM lookup failure blocks deletion",
			cacheErr: errors.New("cache unavailable"),
			wantErr:  "failed to get VMs using image PVC default/image: cache unavailable",
			reason:   metav1.StatusReasonInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmCache := newImageVMCache()
			vmCache.err = tt.cacheErr
			for _, vm := range tt.vms {
				assert.NoError(t, vmCache.indexer.Add(vm))
			}
			clientset := fake.NewSimpleClientset(imagePVC("default", "image", "shared-sc"))
			for _, pvc := range tt.pvcs {
				assert.NoError(t, clientset.Tracker().Add(pvc))
			}
			for _, dv := range tt.dvs {
				assert.NoError(t, clientset.Tracker().Add(dv))
			}
			validator := NewValidator(
				fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.PodCache(clientset.CoreV1().Pods),
				fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				nil, nil, nil, vmCache,
				fakeclients.DataVolumeCache(func(string) cditype.DataVolumeInterface {
					return clientset.CdiV1beta1().DataVolumes()
				}),
				nil,
			)

			vmImage := image.DeepCopy()
			if tt.emptySC {
				vmImage.Status.StorageClassName = ""
			}
			err := validator.Delete(nil, vmImage)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
			var admitErr werror.AdmitError
			if assert.ErrorAs(t, err, &admitErr) {
				assert.Equal(t, tt.reason, admitErr.AsResult().Reason)
			}
		})
	}
}

func TestDeleteBackingImage(t *testing.T) {
	tests := []struct {
		name    string
		pvc     *corev1.PersistentVolumeClaim
		vm      *kubevirtv1.VirtualMachine
		emptySC bool
		listErr error
		wantErr string
	}{
		{name: "unused backing image"},
		{
			name:    "PVC uses the image storage class",
			pvc:     imagePVC("default", "clone", "image-sc"),
			wantErr: "Cannot delete image default/image: being used by volume default/clone",
		},
		{
			name:    "PVC in another namespace uses the image storage class",
			pvc:     imagePVC("other", "clone", "image-sc"),
			wantErr: "Cannot delete image default/image: being used by volume other/clone",
		},
		{
			name: "VM uses an unrelated PVC with the same name as the image",
			pvc:  imagePVC("default", "image", "other-sc"),
			vm:   imageVM("default", "image"),
		},
		{
			name: "PVC without a storage class",
			pvc:  &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "volume", Namespace: "default"}},
		},
		{
			name:    "uninitialized image does not match PVCs with an empty storage class",
			pvc:     imagePVC("default", "volume", ""),
			emptySC: true,
		},
		{
			name:    "PVC lookup failure blocks deletion",
			listErr: errors.New("cache unavailable"),
			wantErr: "cache unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tt.pvc != nil {
				assert.NoError(t, clientset.Tracker().Add(tt.pvc))
			}
			if tt.listErr != nil {
				clientset.PrependReactor("list", "persistentvolumeclaims",
					func(k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, tt.listErr
					})
			}
			vmCache := newImageVMCache()
			if tt.vm != nil {
				assert.NoError(t, vmCache.indexer.Add(tt.vm))
			}
			validator := NewValidator(
				fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.PodCache(clientset.CoreV1().Pods),
				fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				nil, nil,
				fakeclients.VMBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
				vmCache, nil, nil,
			)
			image := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "image", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "image",
				},
				Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: "image-sc"},
			}
			if tt.emptySC {
				image.Status.StorageClassName = ""
			}

			err := validator.Delete(nil, image)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func imagePVC(namespace, name, storageClass string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &storageClass},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func imageVM(namespace, pvcName string) *kubevirtv1.VirtualMachine {
	runStrategy := kubevirtv1.RunStrategyHalted
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "imported", Namespace: namespace},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Volumes: []kubevirtv1.Volume{{
						Name: "disk",
						VolumeSource: kubevirtv1.VolumeSource{
							PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					}},
				},
			},
		},
	}
}

type imageVMCache struct {
	ctlkv1.VirtualMachineCache
	indexer cache.Indexer
	err     error
}

func newImageVMCache() *imageVMCache {
	return &imageVMCache{
		indexer: cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			indexeres.VMByPVCIndex: func(obj interface{}) ([]string, error) {
				return indexeres.VMByPVC(obj.(*kubevirtv1.VirtualMachine))
			},
		}),
	}
}

func (c *imageVMCache) GetByIndex(index, key string) ([]*kubevirtv1.VirtualMachine, error) {
	if c.err != nil {
		return nil, c.err
	}
	objects, err := c.indexer.ByIndex(index, key)
	if err != nil {
		return nil, err
	}
	vms := make([]*kubevirtv1.VirtualMachine, 0, len(objects))
	for _, obj := range objects {
		vms = append(vms, obj.(*kubevirtv1.VirtualMachine))
	}
	return vms, nil
}

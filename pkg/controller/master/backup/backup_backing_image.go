package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

const (
	backupBackingImageControllerName = "harvester-backup-backing-image-controller"
)

type backupBackingImageHandler struct {
	ctx                     context.Context
	secretCache             ctlcorev1.SecretCache
	longhornSettingCache    ctllonghornv1.SettingCache
	settingCache            ctlharvesterv1.SettingCache
	vmImages                ctlharvesterv1.VirtualMachineImageClient
	vmImageCache            ctlharvesterv1.VirtualMachineImageCache
	backingImageCache       ctllonghornv1.BackingImageCache
	backupBackingImageCache ctllonghornv1.BackupBackingImageCache
}

// RegisterBackupBackingImage register the backup backing image controller and resync vmimage metadata when backup backing image is completed
func RegisterBackupBackingImage(ctx context.Context, management *config.Management, _ config.Options) error {
	vmImages := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta2().Setting()
	backingImages := management.LonghornFactory.Longhorn().V1beta2().BackingImage()
	backupBackingImages := management.LonghornFactory.Longhorn().V1beta2().BackupBackingImage()

	backupBackingImageController := &backupBackingImageHandler{
		ctx:                     ctx,
		secretCache:             secrets.Cache(),
		longhornSettingCache:    longhornSettings.Cache(),
		settingCache:            settings.Cache(),
		vmImages:                vmImages,
		vmImageCache:            vmImages.Cache(),
		backingImageCache:       backingImages.Cache(),
		backupBackingImageCache: backupBackingImages.Cache(),
	}

	backupBackingImages.OnChange(ctx, backupBackingImageControllerName, backupBackingImageController.OnBackupBackingImageChange)
	return nil
}

// OnBackupBackingImageChange resync vmimage metadata files when backup backing image is completed
func (h *backupBackingImageHandler) OnBackupBackingImageChange(_ string, backupBackingImage *lhv1beta2.BackupBackingImage) (*lhv1beta2.BackupBackingImage, error) {
	if backupBackingImage == nil || backupBackingImage.DeletionTimestamp != nil {
		return nil, nil
	}

	if backupBackingImage.Status.State != lhv1beta2.BackupStateCompleted {
		return nil, nil
	}

	backingImage, err := h.backingImageCache.Get(backupBackingImage.Namespace, backupBackingImage.Status.BackingImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	vmImageNamespace, vmImageName := ref.Parse(backingImage.Annotations[util.AnnotationImageID])
	if vmImageNamespace == "" || vmImageName == "" {
		return nil, nil
	}

	vmImage, err := h.vmImageCache.Get(vmImageNamespace, vmImageName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return nil, err
	}

	// when backup target is reset to default, do not trig sync
	if target.IsDefaultBackupTarget() {
		return nil, nil
	}

	bsDriver, err := backuputil.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return nil, err
	}

	vmImageMetadata := &VirtualMachineImageMetadata{
		Name:                   vmImage.Name,
		Namespace:              vmImage.Namespace,
		URL:                    backupBackingImage.Status.URL,
		Checksum:               backupBackingImage.Status.Checksum,
		DisplayName:            vmImage.Spec.DisplayName,
		Description:            vmImage.Spec.Description,
		StorageClassParameters: vmImage.Spec.StorageClassParameters,
	}
	if vmImageMetadata.Namespace == "" {
		vmImageMetadata.Namespace = metav1.NamespaceDefault
	}

	data, err := json.Marshal(vmImageMetadata)
	if err != nil {
		return nil, err
	}

	shouldUpload := true
	destPath := backuputil.GetVMImageMetadataFilePath(vmImage.Namespace, vmImage.Name)
	if bsDriver.FileExists(destPath) {
		if remoteVMImageMetadata, err := loadVMImageMetadataInBackupTarget(destPath, bsDriver); err != nil {
			return nil, err
		} else if reflect.DeepEqual(vmImageMetadata, remoteVMImageMetadata) {
			shouldUpload = false
		}
	}

	if shouldUpload {
		logrus.Debugf("upload vm image metadata %s/%s to backup target %s", vmImage.Namespace, vmImage.Name, target.Type)
		if err := bsDriver.Write(destPath, bytes.NewReader(data)); err != nil {
			return nil, err
		}

		vmImageCopy := vmImage.DeepCopy()
		harvesterv1.MetadataReady.True(vmImageCopy)
		vmImageCopy.Status.BackupTarget = &harvesterv1.BackupTarget{
			Endpoint:     target.Endpoint,
			BucketName:   target.BucketName,
			BucketRegion: target.BucketRegion,
		}
		if !reflect.DeepEqual(vmImage, vmImageCopy) {
			_, err = h.vmImages.Update(vmImageCopy)
			return nil, err
		}
	}
	return nil, nil
}

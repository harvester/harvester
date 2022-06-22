package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"

	"github.com/longhorn/backupstore"
	lhcontroller "github.com/longhorn/longhorn-manager/controller"
	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhmanager "github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	lhv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	optionBackingImageName = "backingImage"
	optionMigratable       = "migratable"
)

// vmImageHandler syncs status on vm image changes, and manage a storageclass & a backingimage per vm image
type vmImageHandler struct {
	httpClient     http.Client
	storageClasses v1.StorageClassClient
	images         ctlharvesterv1.VirtualMachineImageClient
	backingImages  lhv1beta1.BackingImageClient
	pvcCache       ctlcorev1.PersistentVolumeClaimCache
	secretCache    ctlcorev1.SecretCache
}

func (h *vmImageHandler) OnChanged(_ string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil || image.DeletionTimestamp != nil {
		return image, nil
	}
	if harvesterv1.ImageInitialized.GetStatus(image) == "" {
		return h.initialize(image)
	} else if image.Spec.URL != image.Status.AppliedURL {
		// URL is changed, recreate the storageclass and backingimage
		if err := h.backingImages.Delete(util.LonghornSystemNamespaceName, getBackingImageName(image), &metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return image, err
		}
		if err := h.storageClasses.Delete(getImageStorageClassName(image.Name), &metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return image, err
		}
		return h.initialize(image)
	}

	if harvesterv1.ImageImported.GetStatus(image) == string(corev1.ConditionTrue) {
		if err := h.checkImageFilesInBackupTarget(*image); err != nil {
			return image, err
		}
	}
	return image, nil
}

func (h *vmImageHandler) OnRemove(_ string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}
	scName := getImageStorageClassName(image.Name)
	if err := h.storageClasses.Delete(scName, &metav1.DeleteOptions{}); !errors.IsNotFound(err) && err != nil {
		return image, err
	}
	biName := getBackingImageName(image)
	if err := h.backingImages.Delete(util.LonghornSystemNamespaceName, biName, &metav1.DeleteOptions{}); !errors.IsNotFound(err) && err != nil {
		return image, err
	}
	return image, nil
}

func (h *vmImageHandler) initialize(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := h.createBackingImage(image); err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}
	if err := h.createStorageClass(image); err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	toUpdate := image.DeepCopy()
	toUpdate.Status.AppliedURL = toUpdate.Spec.URL
	toUpdate.Status.StorageClassName = getImageStorageClassName(image.Name)

	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeDownload {
		resp, err := h.httpClient.Head(image.Spec.URL)
		if err != nil {
			harvesterv1.ImageInitialized.False(toUpdate)
			harvesterv1.ImageInitialized.Message(toUpdate, err.Error())
			return h.images.Update(toUpdate)
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			harvesterv1.ImageInitialized.False(toUpdate)
			harvesterv1.ImageInitialized.Message(toUpdate, fmt.Sprintf("got %d status code from %s", resp.StatusCode, image.Spec.URL))
			return h.images.Update(toUpdate)
		}

		if resp.ContentLength > 0 {
			toUpdate.Status.Size = resp.ContentLength
		}
	} else {
		toUpdate.Status.Progress = 0
	}

	harvesterv1.ImageImported.Unknown(toUpdate)
	harvesterv1.ImageImported.Reason(toUpdate, "Importing")
	harvesterv1.ImageInitialized.True(toUpdate)
	harvesterv1.ImageInitialized.Reason(toUpdate, "Initialized")

	return h.images.Update(toUpdate)
}

func (h *vmImageHandler) createBackingImage(image *harvesterv1.VirtualMachineImage) error {
	bi := &v1beta1.BackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getBackingImageName(image),
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.AnnotationImageID: ref.Construct(image.Namespace, image.Name),
			},
		},
		Spec: v1beta1.BackingImageSpec{
			SourceType:       v1beta1.BackingImageDataSourceType(image.Spec.SourceType),
			SourceParameters: map[string]string{},
			Checksum:         image.Spec.Checksum,
		},
	}
	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeDownload {
		bi.Spec.SourceParameters[v1beta1.DataSourceTypeDownloadParameterURL] = image.Spec.URL
	}

	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeExportVolume {
		pvc, err := h.pvcCache.Get(image.Spec.PVCNamespace, image.Spec.PVCName)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", image.Spec.PVCName, image.Namespace, err.Error())
		}

		bi.Spec.SourceParameters[lhcontroller.DataSourceTypeExportFromVolumeParameterVolumeName] = pvc.Spec.VolumeName
		bi.Spec.SourceParameters[lhmanager.DataSourceTypeExportFromVolumeParameterExportType] = lhmanager.DataSourceTypeExportFromVolumeParameterExportTypeRAW
	}

	_, err := h.backingImages.Create(bi)
	return err
}

func (h *vmImageHandler) createStorageClass(image *harvesterv1.VirtualMachineImage) error {
	recliamPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: getImageStorageClassName(image.Name),
		},
		Provisioner:          types.LonghornDriverName,
		ReclaimPolicy:        &recliamPolicy,
		AllowVolumeExpansion: pointer.BoolPtr(true),
		VolumeBindingMode:    &volumeBindingMode,
		Parameters: map[string]string{
			types.OptionNumberOfReplicas:    "3",
			types.OptionStaleReplicaTimeout: "30",
			optionMigratable:                "true",
			optionBackingImageName:          getBackingImageName(image),
		},
	}

	_, err := h.storageClasses.Create(sc)
	return err
}

func (h *vmImageHandler) checkImageFilesInBackupTarget(image harvesterv1.VirtualMachineImage) error {
	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return err
	}

	if target.IsDefaultBackupTarget() {
		return nil
	}

	if target.Type == settings.S3BackupType {
		secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
		if err != nil {
			return err
		}
		os.Setenv(backup.AWSAccessKey, string(secret.Data[backup.AWSAccessKey]))
		os.Setenv(backup.AWSSecretKey, string(secret.Data[backup.AWSSecretKey]))
		os.Setenv(backup.AWSEndpoints, string(secret.Data[backup.AWSEndpoints]))
		os.Setenv(backup.AWSCERT, string(secret.Data[backup.AWSCERT]))
	}

	bsDriver, err := backupstore.GetBackupStoreDriver(backup.ConstructEndpoint(target))
	if err != nil {
		return err
	}

	if err = h.uploadImageMetadataToBackupTarget(bsDriver, image); err != nil {
		return err
	}

	if err = h.uploadImageISOToBackupTarget(bsDriver, image); err != nil {
		return err
	}

	return nil
}

func (h *vmImageHandler) uploadImageMetadataToBackupTarget(bsDriver backupstore.BackupStoreDriver, image harvesterv1.VirtualMachineImage) error {
	vmImageMetadata := &VirtualMachineImageMetadata{
		Name:             image.Name,
		Namespace:        image.Namespace,
		Description:      image.Spec.Description,
		DisplayName:      image.Spec.DisplayName,
		SourceType:       image.Spec.SourceType,
		PVCName:          image.Spec.PVCName,
		PVCNamespace:     image.Spec.PVCNamespace,
		URL:              image.Spec.URL,
		Checksum:         image.Spec.Checksum,
		Size:             image.Status.Size,
		StorageClassName: image.Status.StorageClassName,
	}

	vmImageMetadataJsonStr, err := json.Marshal(vmImageMetadata)
	if err != nil {
		return fmt.Errorf("cannot marshal vm image metadata %+v, err: %w", vmImageMetadata, err)
	}

	shouldUpload := true
	destURL := getVMImageMetadataFilePath(image)
	if bsDriver.FileExists(destURL) {
		if remoteVMImageMetadata, err := loadVMImageMetadataInBackupTarget(destURL, bsDriver); err != nil {
			return err
		} else if reflect.DeepEqual(vmImageMetadata, remoteVMImageMetadata) {
			shouldUpload = false
		}
	}

	if shouldUpload {
		logrus.Debugf("upload vm image metadata %s/%s to backup target %s in %s", image.Namespace, image.Name, bsDriver.Kind(), destURL)
		if err := bsDriver.Write(destURL, bytes.NewReader(vmImageMetadataJsonStr)); err != nil {
			return fmt.Errorf("cannot upload vm image metadata %+v to backup target, err: %w", vmImageMetadata, err)
		}
	}
	return nil
}

func (h *vmImageHandler) uploadImageISOToBackupTarget(bsDriver backupstore.BackupStoreDriver, image harvesterv1.VirtualMachineImage) error {
	destURL := getVMImageISOFilePath(image)
	if bsDriver.FileExists(destURL) && bsDriver.FileSize(destURL) == image.Status.Size {
		return nil
	}

	// download image from LH
	res, err := http.Get(fmt.Sprintf("http://longhorn-backend.longhorn-system:9500/v1/backingimages/%s/download", getBackingImageName(&image)))
	if err != nil {
		return fmt.Errorf("cannot download backing image %s from LH", getBackingImageName(&image))
	}
	defer res.Body.Close()
	imageISOBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("cannot read backing image %s response from LH", getBackingImageName(&image))
	}

	// upload image iso to backup target
	if err := bsDriver.Write(destURL, bytes.NewReader(imageISOBytes)); err != nil {
		return fmt.Errorf("cannot upload vm image %s/%s to backup target, err: %w", image.Namespace, image.Name, err)
	}
	return nil
}

func getImageStorageClassName(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}

func getBackingImageName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", image.Namespace, image.Name)
}

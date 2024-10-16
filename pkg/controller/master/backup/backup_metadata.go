package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/longhorn/backupstore"

	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	vmBackupMetadataFolderPath   = "harvester/vmbackups/"
	backupMetadataControllerName = "harvester-backup-metadata-controller"
)

type VirtualMachineImageMetadata struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	URL         string `json:"url"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	// Checksum is from the backup backing image status
	Checksum               string            `json:"checksum,omitempty"`
	StorageClassParameters map[string]string `json:"storageClassParameters,omitempty"`
}

type VirtualMachineBackupMetadata struct {
	Name          string                                `json:"name"`
	Namespace     string                                `json:"namespace"`
	BackupSpec    harvesterv1.VirtualMachineBackupSpec  `json:"backupSpec,omitempty"`
	VMSourceSpec  *harvesterv1.VirtualMachineSourceSpec `json:"vmSourceSpec,omitempty"`
	VolumeBackups []harvesterv1.VolumeBackup            `json:"volumeBackups,omitempty"`
	SecretBackups []harvesterv1.SecretBackup            `json:"secretBackups,omitempty"`
}

type MetadataHandler struct {
	ctx                  context.Context
	namespaces           ctlcorev1.NamespaceClient
	namespaceCache       ctlcorev1.NamespaceCache
	secretCache          ctlcorev1.SecretCache
	vms                  ctlkubevirtv1.VirtualMachineController
	longhornSettingCache ctllonghornv1.SettingCache
	settings             ctlharvesterv1.SettingController
	vmBackups            ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache        ctlharvesterv1.VirtualMachineBackupCache
	vmImages             ctlharvesterv1.VirtualMachineImageClient
	vmImageCache         ctlharvesterv1.VirtualMachineImageCache
	storageClassCache    ctlstoragev1.StorageClassCache
}

// RegisterBackupMetadata register the setting controller and resync vm backup metadata when backup target change
func RegisterBackupMetadata(ctx context.Context, management *config.Management, _ config.Options) error {
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	vmImages := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	namespaces := management.CoreFactory.Core().V1().Namespace()
	secrets := management.CoreFactory.Core().V1().Secret()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta2().Setting()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	storageClass := management.StorageFactory.Storage().V1().StorageClass()

	backupMetadataController := &MetadataHandler{
		ctx:                  ctx,
		namespaces:           namespaces,
		namespaceCache:       namespaces.Cache(),
		secretCache:          secrets.Cache(),
		vms:                  vms,
		longhornSettingCache: longhornSettings.Cache(),
		settings:             settings,
		vmBackups:            vmBackups,
		vmBackupCache:        vmBackups.Cache(),
		vmImages:             vmImages,
		vmImageCache:         vmImages.Cache(),
		storageClassCache:    storageClass.Cache(),
	}

	settings.OnChange(ctx, backupMetadataControllerName, backupMetadataController.OnBackupTargetChange)
	return nil
}

// OnBackupTargetChange resync vm metadata files when backup target change
func (h *MetadataHandler) OnBackupTargetChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil ||
		setting.Name != settings.BackupTargetSettingName || setting.Value == "" {
		return nil, nil
	}

	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return setting, err
	}

	logrus.Debugf("backup target change, sync vm backup:%s:%s", target.Type, target.Endpoint)

	// when backup target is reset to default, do not trig sync
	if target.IsDefaultBackupTarget() {
		return nil, nil
	}

	contextLogger := logrus.WithFields(logrus.Fields{
		"target.type":     target.Type,
		"target.endpoint": target.Endpoint,
	})
	contextLogger.Info("start syncing vm image metadata...")
	if err = h.syncVMImage(target); err != nil {
		contextLogger.WithError(err).Errorf("can't sync vm image metadata")
		h.settings.EnqueueAfter(setting.Name, 5*time.Second)
		return nil, nil
	}

	contextLogger.Info("start syncing vm backup metadata...")
	if err = h.syncVMBackup(target); err != nil {
		contextLogger.WithError(err).Errorf("can't sync vm backup metadata")
		h.settings.EnqueueAfter(setting.Name, 5*time.Second)
		return nil, nil
	}

	return nil, nil
}

func (h *MetadataHandler) syncVMImage(target *settings.BackupTarget) error {
	bsDriver, err := util.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	namespaceFolders, err := bsDriver.List(filepath.Join(util.VMImageMetadataFolderPath))
	if err != nil {
		return err
	}

	for _, namespaceFolder := range namespaceFolders {
		fileNames, err := bsDriver.List(filepath.Join(util.VMImageMetadataFolderPath, namespaceFolder))
		if err != nil {
			return err
		}
		for _, fileName := range fileNames {
			imageMetadata, err := loadVMImageMetadataInBackupTarget(filepath.Join(util.VMImageMetadataFolderPath, namespaceFolder, fileName), bsDriver)
			if err != nil {
				return err
			}
			if imageMetadata.Namespace == "" {
				imageMetadata.Namespace = metav1.NamespaceDefault
			}
			if err := h.createVMImageIfNotExist(*imageMetadata); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *MetadataHandler) createVMImageIfNotExist(imageMetadata VirtualMachineImageMetadata) error {
	if _, err := h.vmImageCache.Get(imageMetadata.Namespace, imageMetadata.Name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
	}

	if sameDisplayNameImages, err := h.vmImageCache.List(imageMetadata.Namespace, labels.SelectorFromSet(map[string]string{
		util.LabelImageDisplayName: imageMetadata.DisplayName,
	})); err != nil {
		return err
	} else if len(sameDisplayNameImages) > 0 {
		logrus.WithFields(logrus.Fields{
			"namespace":   imageMetadata.Namespace,
			"name":        imageMetadata.Name,
			"displayName": imageMetadata.DisplayName,
		}).Warn("skip create vm image, because there is already an image with the same display name")
		return nil
	}

	if err := h.createNamespaceIfNotExist(imageMetadata.Namespace); err != nil {
		return err
	}

	if _, err := h.vmImages.Create(&harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageMetadata.Name,
			Namespace: imageMetadata.Namespace,
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			SourceType:             harvesterv1.VirtualMachineImageSourceTypeRestore,
			URL:                    imageMetadata.URL,
			Description:            imageMetadata.Description,
			DisplayName:            imageMetadata.DisplayName,
			Checksum:               imageMetadata.Checksum,
			StorageClassParameters: imageMetadata.StorageClassParameters,
		},
	}); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"namespace":   imageMetadata.Namespace,
		"name":        imageMetadata.Name,
		"displayName": imageMetadata.DisplayName,
	}).Info("create vm image from backup target")
	return nil
}

func (h *MetadataHandler) syncVMBackup(target *settings.BackupTarget) error {
	bsDriver, err := util.GetBackupStoreDriver(h.secretCache, target)
	if err != nil {
		return err
	}

	fileNames, err := bsDriver.List(filepath.Join(vmBackupMetadataFolderPath))
	if err != nil {
		return err
	}

	namespaceFolderSet := map[string]bool{} // ignore value of map, we only use it as a set
	requiredMovingFilePaths := []string{}
	for _, fileName := range fileNames {
		filePath := filepath.Join(vmBackupMetadataFolderPath, fileName)
		if bsDriver.FileExists(filePath) {
			requiredMovingFilePaths = append(requiredMovingFilePaths, filePath)
			continue
		}
		namespaceFolderSet[filePath] = true
	}

	if err = h.moveFilePaths(requiredMovingFilePaths, bsDriver, namespaceFolderSet); err != nil {
		return err
	}

	vmbackupMetadataFilePaths := []string{}
	for namespaceFolder := range namespaceFolderSet {
		fileNames, err := bsDriver.List(namespaceFolder)
		if err != nil {
			return err
		}

		for _, fileName := range fileNames {
			filePath := filepath.Join(namespaceFolder, fileName)
			if bsDriver.FileExists(filePath) {
				vmbackupMetadataFilePaths = append(vmbackupMetadataFilePaths, filePath)
			}
		}
	}

	return h.loadBackupMetadataAndCreateVMBackup(target, vmbackupMetadataFilePaths, bsDriver)
}

func (h *MetadataHandler) createVMBackupIfNotExist(backupMetadata VirtualMachineBackupMetadata, target *settings.BackupTarget) error {
	if _, err := h.vmBackupCache.Get(backupMetadata.Namespace, backupMetadata.Name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
	}

	for _, volumeBackup := range backupMetadata.VolumeBackups {
		if volumeBackup.PersistentVolumeClaim.Spec.StorageClassName == nil {
			continue
		}
		if _, err := h.storageClassCache.Get(*volumeBackup.PersistentVolumeClaim.Spec.StorageClassName); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"namespace":     backupMetadata.Namespace,
				"name":          backupMetadata.Name,
				"storageClasss": *volumeBackup.PersistentVolumeClaim.Spec.StorageClassName,
			}).Warn("skip creating vm backup, because the storage class is not found")
			return nil
		}
	}

	if err := h.createNamespaceIfNotExist(backupMetadata.Namespace); err != nil {
		return err
	}
	if _, err := h.vmBackups.Create(&harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupMetadata.Name,
			Namespace: backupMetadata.Namespace,
		},
		Spec: backupMetadata.BackupSpec,
		Status: &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.BoolPtr(false),
			BackupTarget: &harvesterv1.BackupTarget{
				Endpoint:     target.Endpoint,
				BucketName:   target.BucketName,
				BucketRegion: target.BucketRegion,
			},
			SourceSpec:    backupMetadata.VMSourceSpec,
			VolumeBackups: backupMetadata.VolumeBackups,
			SecretBackups: backupMetadata.SecretBackups,
		},
	}); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"namespace": backupMetadata.Namespace,
		"name":      backupMetadata.Name,
	}).Info("create vm backup from backup target")
	return nil
}

func (h *MetadataHandler) createNamespaceIfNotExist(namespace string) error {
	if _, err := h.namespaceCache.Get(namespace); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
	}

	_, err := h.namespaces.Create(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	return err
}

func (h *MetadataHandler) loadBackupMetadataAndCreateVMBackup(target *settings.BackupTarget, filePaths []string, bsDriver backupstore.BackupStoreDriver) error {
	for _, filePath := range filePaths {
		backupMetadata, err := loadBackupMetadataInBackupTarget(filePath, bsDriver)
		if err != nil {
			return err
		}
		if backupMetadata.Namespace == "" {
			backupMetadata.Namespace = metav1.NamespaceDefault
		}
		if err := h.createVMBackupIfNotExist(*backupMetadata, target); err != nil {
			return err
		}
	}
	return nil
}

func (h *MetadataHandler) moveFilePaths(filePaths []string, bsDriver backupstore.BackupStoreDriver, namespaceFolderSet map[string]bool) error {
	for _, filePath := range filePaths {
		backupMetadata, err := loadBackupMetadataInBackupTarget(filePath, bsDriver)
		if err != nil {
			return err
		}
		if backupMetadata.Namespace == "" {
			backupMetadata.Namespace = metav1.NamespaceDefault
		}

		namespaceFolderSet[backupMetadata.Namespace] = true

		j, err := json.Marshal(backupMetadata)
		if err != nil {
			return err
		}

		newFilePath := getVMBackupMetadataFilePath(backupMetadata.Namespace, backupMetadata.Name)
		logrus.Infof("move vm backup metadata %s/%s from %s to %s", backupMetadata.Namespace, backupMetadata.Name, filePath, newFilePath)
		if err = bsDriver.Write(newFilePath, bytes.NewReader(j)); err != nil {
			return err
		}
		if err = bsDriver.Remove(filePath); err != nil {
			return err
		}
	}
	return nil
}

func loadVMImageMetadataInBackupTarget(filePath string, bsDriver backupstore.BackupStoreDriver) (*VirtualMachineImageMetadata, error) {
	if !bsDriver.FileExists(filePath) {
		return nil, fmt.Errorf("cannot find %v in backupstore", filePath)
	}

	rc, err := bsDriver.Read(filePath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	imageMetadata := &VirtualMachineImageMetadata{}
	if err := json.NewDecoder(rc).Decode(imageMetadata); err != nil {
		return nil, err
	}
	return imageMetadata, nil
}

func loadBackupMetadataInBackupTarget(filePath string, bsDriver backupstore.BackupStoreDriver) (*VirtualMachineBackupMetadata, error) {
	if !bsDriver.FileExists(filePath) {
		return nil, fmt.Errorf("cannot find %v in backupstore", filePath)
	}

	rc, err := bsDriver.Read(filePath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	backupMetadata := &VirtualMachineBackupMetadata{}
	if err := json.NewDecoder(rc).Decode(backupMetadata); err != nil {
		return nil, err
	}
	return backupMetadata, nil
}

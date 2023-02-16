package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/longhorn/backupstore"
	// Although we don't use following drivers directly, we need to import them to register drivers.
	// NFS Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/nfs/nfs.go#L47-L51
	// S3 Ref: https://github.com/longhorn/backupstore/blob/3912081eb7c5708f0027ebbb0da4934537eb9d72/s3/s3.go#L33-L37
	_ "github.com/longhorn/backupstore/nfs" //nolint
	_ "github.com/longhorn/backupstore/s3"  //nolint
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	metadataFolderPath           = "harvester/vmbackups/"
	backupMetadataControllerName = "harvester-backup-metadata-controller"
)

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
}

// RegisterBackupMetadata register the setting controller and resync vm backup metadata when backup target change
func RegisterBackupMetadata(ctx context.Context, management *config.Management, opts config.Options) error {
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	namespaces := management.CoreFactory.Core().V1().Namespace()
	secrets := management.CoreFactory.Core().V1().Secret()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta1().Setting()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()

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
	}

	settings.OnChange(ctx, backupMetadataControllerName, backupMetadataController.OnBackupTargetChange)
	return nil
}

// OnBackupTargetChange resync vm metadata files when backup target change
func (h *MetadataHandler) OnBackupTargetChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
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

	if err = h.syncVMBackup(target); err != nil {
		logrus.Errorf("can't sync vm backup metadata, target:%s:%s, err: %v", target.Type, target.Endpoint, err)
		h.settings.EnqueueAfter(setting.Name, 5*time.Second)
		return nil, nil
	}

	return nil, nil
}

func (h *MetadataHandler) syncVMBackup(target *settings.BackupTarget) error {
	if target.Type == settings.S3BackupType {
		secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
		if err != nil {
			return err
		}
		os.Setenv(AWSAccessKey, string(secret.Data[AWSAccessKey]))
		os.Setenv(AWSSecretKey, string(secret.Data[AWSSecretKey]))
		os.Setenv(AWSEndpoints, string(secret.Data[AWSEndpoints]))
		os.Setenv(AWSCERT, string(secret.Data[AWSCERT]))
	}

	endpoint := ConstructEndpoint(target)
	bsDriver, err := backupstore.GetBackupStoreDriver(endpoint)
	if err != nil {
		return err
	}

	fileNames, err := bsDriver.List(filepath.Join(metadataFolderPath))
	if err != nil {
		return err
	}

	for _, fileName := range fileNames {
		backupMetadata, err := loadBackupMetadataInBackupTarget(filepath.Join(metadataFolderPath, fileName), bsDriver)
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

func (h *MetadataHandler) createVMBackupIfNotExist(backupMetadata VirtualMachineBackupMetadata, target *settings.BackupTarget) error {
	if _, err := h.vmBackupCache.Get(backupMetadata.Namespace, backupMetadata.Name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
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

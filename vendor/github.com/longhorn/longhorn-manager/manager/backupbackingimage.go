package manager

import (
	"fmt"

	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) ListBackupBackingImagesSorted() ([]*longhorn.BackupBackingImage, error) {
	backupBackingImageMap, err := m.ds.ListBackupBackingImages()
	if err != nil {
		return []*longhorn.BackupBackingImage{}, err
	}

	backupBackingImageNames, err := util.SortKeys(backupBackingImageMap)
	if err != nil {
		return []*longhorn.BackupBackingImage{}, err
	}

	backupBackingImages := make([]*longhorn.BackupBackingImage, len(backupBackingImageMap))
	for i, backupBackingImageName := range backupBackingImageNames {
		backupBackingImages[i] = backupBackingImageMap[backupBackingImageName]
	}

	return backupBackingImages, nil
}

func (m *VolumeManager) GetBackupBackingImage(name string) (*longhorn.BackupBackingImage, error) {
	return m.ds.GetBackupBackingImageRO(name)
}

func (m *VolumeManager) DeleteBackupBackingImage(name string) error {
	return m.ds.DeleteBackupBackingImage(name)
}

func (m *VolumeManager) RestoreBackupBackingImage(name, secret, secretNamespace, dataEngine string) error {
	if name == "" || dataEngine == "" {
		return fmt.Errorf("missing parameters for restoring backing image, name=%v dataEngine=%v", name, dataEngine)
	}

	bbi, err := m.ds.GetBackupBackingImageRO(name)
	if err != nil {
		return err
	}
	biName := bbi.Spec.BackingImage
	bi, err := m.ds.GetBackingImageRO(biName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get backing image %v to check if it exists", biName)
		}
	}

	if bi != nil {
		return fmt.Errorf("backing image %v already exists", biName)
	}

	return m.restoreBackingImage(bbi.Spec.BackupTargetName, biName, secret, secretNamespace, dataEngine)
}

func (m *VolumeManager) CreateBackupBackingImage(name, backingImageName, backupTargetName string) error {
	_, err := m.ds.GetBackingImageRO(backingImageName)
	if err != nil {
		return errors.Wrapf(err, "failed to get backing image %v", backingImageName)
	}

	backupBackingImage, err := m.ds.GetBackupBackingImageRO(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check if backup backing image %v exists", name)
		}
	}

	if backupBackingImage != nil {
		return fmt.Errorf("backup backing image %v already exists", name)
	}

	btName := backupTargetName
	if backupTargetName == "" {
		btName = types.DefaultBackupTargetName
	}

	backupBackingImage = &longhorn.BackupBackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: longhorn.BackupBackingImageSpec{
			UserCreated:      true,
			BackingImage:     backingImageName,
			BackupTargetName: btName,
		},
	}
	if _, err = m.ds.CreateBackupBackingImage(backupBackingImage); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create backup backing image %s in the cluster", name)
	}

	return nil
}

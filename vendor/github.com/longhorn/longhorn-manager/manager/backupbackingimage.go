package manager

import (
	"fmt"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/util"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (m *VolumeManager) RestoreBackupBackingImage(name string, secret, secretNamespace string) error {
	if name == "" {
		return fmt.Errorf("restore backing image name is not given")
	}
	bi, err := m.ds.GetBackingImageRO(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get backing image %v to check if it exists", name)
		}
	}

	if bi != nil {
		return errors.Wrapf(err, "backing image %v already exists", name)
	}

	return m.restoreBackingImage(name, secret, secretNamespace)
}

func (m *VolumeManager) CreateBackupBackingImage(name string) error {
	_, err := m.ds.GetBackingImageRO(name)
	if err != nil {
		return errors.Wrapf(err, "failed to get backing image %v", name)
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

	backupBackingImage = &longhorn.BackupBackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: longhorn.BackupBackingImageSpec{
			UserCreated: true,
		},
	}
	if _, err = m.ds.CreateBackupBackingImage(backupBackingImage); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create backup backing image %s in the cluster", name)
	}

	return nil
}

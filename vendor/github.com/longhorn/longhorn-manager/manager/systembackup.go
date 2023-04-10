package manager

import (
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) CreateSystemBackup(name string) (*longhorn.SystemBackup, error) {
	logrus.WithField("systemBackup", name).Info("Creating SystemBackup")

	return m.ds.CreateSystemBackup(
		&longhorn.SystemBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	)
}

func (m *VolumeManager) DeleteSystemBackup(name string) error {
	logrus.WithField("systemBackup", name).Info("Deleting SystemBackup")

	err := m.ds.DeleteSystemBackup(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (m *VolumeManager) GetSystemBackup(name string) (*longhorn.SystemBackup, error) {
	return m.ds.GetSystemBackupRO(name)
}

func (m *VolumeManager) ListSystemBackupsSorted() ([]*longhorn.SystemBackup, error) {
	systemBackups, err := m.ds.ListSystemBackups()
	if err != nil {
		return []*longhorn.SystemBackup{}, err
	}

	systemBackupNames, err := util.SortKeys(systemBackups)
	if err != nil {
		return []*longhorn.SystemBackup{}, err
	}

	sortedSystemBackups := make([]*longhorn.SystemBackup, len(systemBackups))
	for i, name := range systemBackupNames {
		sortedSystemBackups[i] = systemBackups[name]
	}
	return sortedSystemBackups, nil
}

package manager

import (
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) CreateSystemRestore(name, systemBackup string) (*longhorn.SystemRestore, error) {
	log := logrus.WithFields(logrus.Fields{
		"systemBackup":  systemBackup,
		"systemRestore": name,
	})
	log.Info("Creating SystemRestore")

	return m.ds.CreateSystemRestore(&longhorn.SystemRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: longhorn.SystemRestoreSpec{
			SystemBackup: systemBackup,
		},
	})
}

func (m *VolumeManager) DeleteSystemRestore(name string) error {
	logrus.WithField("systemRestore", name).Info("Deleting SystemRestore")

	err := m.ds.DeleteSystemRestore(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (m *VolumeManager) GetSystemRestore(name string) (*longhorn.SystemRestore, error) {
	return m.ds.GetSystemRestoreRO(name)
}

func (m *VolumeManager) ListSystemRestoresSorted() ([]*longhorn.SystemRestore, error) {
	systemRestores, err := m.ds.ListSystemRestores()
	if err != nil {
		return []*longhorn.SystemRestore{}, err
	}

	systemRestoreNames, err := util.SortKeys(systemRestores)
	if err != nil {
		return []*longhorn.SystemRestore{}, err
	}

	sortedSystemRestores := make([]*longhorn.SystemRestore, len(systemRestores))
	for i, name := range systemRestoreNames {
		sortedSystemRestores[i] = systemRestores[name]
	}
	return sortedSystemRestores, nil
}

package manager

import (
	"github.com/sirupsen/logrus"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) GetOrphan(name string) (*longhorn.Orphan, error) {
	return m.ds.GetOrphan(name)
}

func (m *VolumeManager) ListOrphans() (map[string]*longhorn.Orphan, error) {
	return m.ds.ListOrphans()
}

func (m *VolumeManager) DeleteOrphan(name string) error {
	if err := m.ds.DeleteOrphan(name); err != nil {
		return err
	}
	logrus.Infof("Deleted orphan %v", name)
	return nil
}

package manager

import (
	"fmt"

	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bsutil "github.com/longhorn/backupstore/util"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (m *VolumeManager) ListSnapshotsCR(volumeName string) (map[string]*longhorn.Snapshot, error) {
	return m.ds.ListVolumeSnapshotsRO(volumeName)
}

func (m *VolumeManager) GetSnapshotCR(snapName string) (*longhorn.Snapshot, error) {
	return m.ds.GetSnapshotRO(snapName)
}

func (m *VolumeManager) DeleteSnapshotCR(snapName string) error {
	return m.ds.DeleteSnapshot(snapName)
}

func (m *VolumeManager) CreateSnapshotCR(snapshotName string, labels map[string]string, volumeName string) (*longhorn.Snapshot, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("volume name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return nil, err
	}

	if snapshotName == "" {
		snapshotName = bsutil.GenerateName("snap")
	}

	snapshotCR := &longhorn.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotName,
		},
		Spec: longhorn.SnapshotSpec{
			Volume:         volumeName,
			CreateSnapshot: true,
			Labels:         labels,
		},
	}

	snapshotCR, err := m.ds.CreateSnapshot(snapshotCR)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Created snapshot CR %v with labels %+v for volume %v", snapshotName, labels, volumeName)
	return snapshotCR, nil
}

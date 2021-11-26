package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/engineapi"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
)

const (
	BackupStatusQueryInterval = 2 * time.Second
)

func (m *VolumeManager) ListSnapshots(volumeName string) (map[string]*longhorn.Snapshot, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("volume name required")
	}
	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return nil, err
	}
	return engine.SnapshotList()
}

func (m *VolumeManager) GetSnapshot(snapshotName, volumeName string) (*longhorn.Snapshot, error) {
	if volumeName == "" || snapshotName == "" {
		return nil, fmt.Errorf("volume and snapshot name required")
	}
	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return nil, err
	}
	snapshot, err := engine.SnapshotGet(snapshotName)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("cannot find snapshot '%s' for volume '%s'", snapshotName, volumeName)
	}
	return snapshot, nil
}

func (m *VolumeManager) CreateSnapshot(snapshotName string, labels map[string]string, volumeName string) (*longhorn.Snapshot, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("volume name required")
	}

	for k, v := range labels {
		if strings.Contains(k, "=") || strings.Contains(v, "=") {
			return nil, fmt.Errorf("labels cannot contain '='")
		}
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return nil, err
	}

	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return nil, err
	}
	snapshotName, err = engine.SnapshotCreate(snapshotName, labels)
	if err != nil {
		return nil, err
	}
	snap, err := engine.SnapshotGet(snapshotName)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("cannot found just created snapshot '%s', for volume '%s'", snapshotName, volumeName)
	}
	logrus.Debugf("Created snapshot %v with labels %+v for volume %v", snapshotName, labels, volumeName)
	return snap, nil
}

func (m *VolumeManager) DeleteSnapshot(snapshotName, volumeName string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return err
	}
	if err := engine.SnapshotDelete(snapshotName); err != nil {
		return err
	}
	logrus.Debugf("Deleted snapshot %v for volume %v", snapshotName, volumeName)
	return nil
}

func (m *VolumeManager) RevertSnapshot(snapshotName, volumeName string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return err
	}
	snapshot, err := engine.SnapshotGet(snapshotName)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("not found snapshot '%s', for volume '%s'", snapshotName, volumeName)
	}
	if err := engine.SnapshotRevert(snapshotName); err != nil {
		return err
	}
	logrus.Debugf("Revert to snapshot %v for volume %v", snapshotName, volumeName)
	return nil
}

func (m *VolumeManager) PurgeSnapshot(volumeName string) error {
	if volumeName == "" {
		return fmt.Errorf("volume name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engine, err := m.GetEngineClient(volumeName)
	if err != nil {
		return err
	}

	if err := engine.SnapshotPurge(); err != nil {
		return err
	}
	logrus.Debugf("Started snapshot purge for volume %v", volumeName)
	return nil
}

func (m *VolumeManager) BackupSnapshot(backupName, volumeName, snapshotName string, labels map[string]string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	backupCR := &longhorn.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name: backupName,
		},
		Spec: longhorn.BackupSpec{
			SnapshotName: snapshotName,
			Labels:       labels,
		},
	}
	_, err := m.ds.CreateBackup(backupCR, volumeName)
	return err
}

func (m *VolumeManager) checkVolumeNotInMigration(volumeName string) error {
	v, err := m.ds.GetVolume(volumeName)
	if err != nil {
		return err
	}
	if v.Spec.MigrationNodeID != "" {
		return fmt.Errorf("cannot operate during migration")
	}
	return nil
}

func (m *VolumeManager) GetEngineClient(volumeName string) (client engineapi.EngineClient, err error) {
	var e *longhorn.Engine

	defer func() {
		err = errors.Wrapf(err, "cannot get client for volume %v", volumeName)
	}()
	es, err := m.ds.ListVolumeEngines(volumeName)
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return nil, fmt.Errorf("cannot find engine")
	}
	if len(es) != 1 {
		return nil, fmt.Errorf("more than one engine exists")
	}
	for _, e = range es {
		break
	}
	if e.Status.CurrentState != longhorn.InstanceStateRunning {
		return nil, fmt.Errorf("engine is not running")
	}
	if isReady, err := m.ds.CheckEngineImageReadiness(e.Status.CurrentImage, m.currentNodeID); !isReady {
		if err != nil {
			return nil, fmt.Errorf("cannot get engine client with image %v: %v", e.Status.CurrentImage, err)
		}
		return nil, fmt.Errorf("cannot get engine client with image %v because it isn't deployed on this node", e.Status.CurrentImage)
	}

	engineCollection := &engineapi.EngineCollection{}
	return engineCollection.NewEngineClient(&engineapi.EngineClientRequest{
		VolumeName:  e.Spec.VolumeName,
		EngineImage: e.Status.CurrentImage,
		IP:          e.Status.IP,
		Port:        e.Status.Port,
	})
}

func (m *VolumeManager) ListBackupTargetsSorted() ([]*longhorn.BackupTarget, error) {
	backupTargetMap, err := m.ds.ListBackupTargets()
	if err != nil {
		return []*longhorn.BackupTarget{}, err
	}
	backupTargetNames, err := sortKeys(backupTargetMap)
	if err != nil {
		return []*longhorn.BackupTarget{}, err
	}
	backupTargets := make([]*longhorn.BackupTarget, len(backupTargetMap))
	for i, backupTargetName := range backupTargetNames {
		backupTargets[i] = backupTargetMap[backupTargetName]
	}
	return backupTargets, nil
}

func (m *VolumeManager) ListBackupVolumes() (map[string]*longhorn.BackupVolume, error) {
	return m.ds.ListBackupVolumes()
}

func (m *VolumeManager) ListBackupVolumesSorted() ([]*longhorn.BackupVolume, error) {
	backupVolumeMap, err := m.ds.ListBackupVolumes()
	if err != nil {
		return []*longhorn.BackupVolume{}, err
	}
	backupVolumeNames, err := sortKeys(backupVolumeMap)
	if err != nil {
		return []*longhorn.BackupVolume{}, err
	}
	backupVolumes := make([]*longhorn.BackupVolume, len(backupVolumeMap))
	for i, backupVolumeName := range backupVolumeNames {
		backupVolumes[i] = backupVolumeMap[backupVolumeName]
	}
	return backupVolumes, nil
}

func (m *VolumeManager) GetBackupVolume(volumeName string) (*longhorn.BackupVolume, error) {
	backupVolume, err := m.ds.GetBackupVolumeRO(volumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the BackupVolume CR is not found, return succeeded result
			// This is to compatible with the Longhorn CSI plugin
			// https://github.com/longhorn/longhorn-manager/blob/v1.1.2/csi/controller_server.go#L446-L455
			return &longhorn.BackupVolume{ObjectMeta: metav1.ObjectMeta{Name: volumeName}}, nil
		}
		return nil, err
	}

	return backupVolume, err
}

func (m *VolumeManager) DeleteBackupVolume(volumeName string) error {
	return m.ds.DeleteBackupVolume(volumeName)
}

func (m *VolumeManager) ListAllBackupsSorted() ([]*longhorn.Backup, error) {
	backupMap, err := m.ds.ListBackups()
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := sortKeys(backupMap)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backups := make([]*longhorn.Backup, len(backupMap))
	for i, backupName := range backupNames {
		backups[i] = backupMap[backupName]
	}
	return backups, nil
}

func (m *VolumeManager) ListBackupsForVolume(volumeName string) (map[string]*longhorn.Backup, error) {
	return m.ds.ListBackupsWithBackupVolumeName(volumeName)
}

func (m *VolumeManager) ListBackupsForVolumeSorted(volumeName string) ([]*longhorn.Backup, error) {
	backupMap, err := m.ListBackupsForVolume(volumeName)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := sortKeys(backupMap)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backups := make([]*longhorn.Backup, len(backupMap))
	for i, backupName := range backupNames {
		backups[i] = backupMap[backupName]
	}
	return backups, nil
}

func (m *VolumeManager) GetBackup(backupName, volumeName string) (*longhorn.Backup, error) {
	return m.ds.GetBackupRO(backupName)
}

func (m *VolumeManager) DeleteBackup(backupName, volumeName string) error {
	return m.ds.DeleteBackup(backupName)
}

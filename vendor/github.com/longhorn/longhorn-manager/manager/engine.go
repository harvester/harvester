package manager

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	BackupStatusQueryInterval = 2 * time.Second

	// minSyncBackupTargetIntervalSec interval in seconds to synchronize backup target to avoid frequent sync
	minSyncBackupTargetIntervalSec = 10
)

func (m *VolumeManager) ListSnapshotInfos(volumeName string) (map[string]*longhorn.SnapshotInfo, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("volume name required")
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return nil, err
	}

	engine, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return nil, err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return nil, err
	}
	defer engineClientProxy.Close()

	return engineClientProxy.SnapshotList(engine)
}

func (m *VolumeManager) GetSnapshotInfo(snapshotName, volumeName string) (*longhorn.SnapshotInfo, error) {
	if volumeName == "" || snapshotName == "" {
		return nil, fmt.Errorf("volume and snapshot name required")
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return nil, err
	}

	engine, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return nil, err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return nil, err
	}
	defer engineClientProxy.Close()

	snapshot, err := engineClientProxy.SnapshotGet(engine, snapshotName)
	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		return nil, fmt.Errorf("cannot find snapshot '%s' for volume '%s'", snapshotName, volumeName)
	}

	return snapshot, nil
}

func (m *VolumeManager) CreateSnapshot(snapshotName string, labels map[string]string, volumeName string) (*longhorn.SnapshotInfo, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("volume name required")
	}

	if err := util.VerifySnapshotLabels(labels); err != nil {
		return nil, err
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return nil, err
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return nil, err
	}

	e, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return nil, err
	}

	freezeFilesystem, err := m.ds.GetFreezeFilesystemForSnapshotSetting(e)
	if err != nil {
		return nil, err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(e, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return nil, err
	}
	defer engineClientProxy.Close()

	snapshotName, err = engineClientProxy.SnapshotCreate(e, snapshotName, labels, freezeFilesystem)
	if err != nil {
		return nil, err
	}

	snap, err := engineClientProxy.SnapshotGet(e, snapshotName)
	if err != nil {
		return nil, err
	}

	if snap == nil {
		return nil, fmt.Errorf("cannot found just created snapshot '%s', for volume '%s'", snapshotName, volumeName)
	}

	logrus.Infof("Created snapshot %v with labels %+v for volume %v", snapshotName, labels, volumeName)
	return snap, nil
}

func (m *VolumeManager) DeleteSnapshot(snapshotName, volumeName string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return err
	}

	engine, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	if err := engineClientProxy.SnapshotDelete(engine, snapshotName); err != nil {
		return err
	}

	logrus.Infof("Deleted snapshot %v for volume %v", snapshotName, volumeName)
	return nil
}

func (m *VolumeManager) RevertSnapshot(snapshotName, volumeName string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return err
	}

	engine, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	snapshot, err := engineClientProxy.SnapshotGet(engine, snapshotName)
	if err != nil {
		return err
	}

	if snapshot == nil {
		return fmt.Errorf("not found snapshot '%s', for volume '%s'", snapshotName, volumeName)
	}

	if snapshot.Removed {
		return fmt.Errorf("not revert to snapshot '%s' for volume '%s' since it's marked as Removed", snapshotName, volumeName)
	}

	if err := engineClientProxy.SnapshotRevert(engine, snapshotName); err != nil {
		return err
	}

	logrus.Infof("Reverted to snapshot %v for volume %v", snapshotName, volumeName)
	return nil
}

func (m *VolumeManager) PurgeSnapshot(volumeName string) error {
	if volumeName == "" {
		return fmt.Errorf("volume name required")
	}

	disablePurge, err := m.ds.GetSettingAsBool(types.SettingNameDisableSnapshotPurge)
	if err != nil {
		return err
	}
	if disablePurge {
		return errors.Errorf("cannot purge snapshots while %v setting is true", types.SettingNameDisableSnapshotPurge)
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	engineCliClient, err := engineapi.GetEngineBinaryClient(m.ds, volumeName, m.currentNodeID)
	if err != nil {
		return err
	}

	engine, err := m.GetRunningEngineByVolume(volumeName)
	if err != nil {
		return err
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(engine, engineCliClient, m.ds, nil, m.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()

	if err := engineClientProxy.SnapshotPurge(engine); err != nil {
		return err
	}

	logrus.Infof("Started snapshot purge for volume %v", volumeName)
	return nil
}

func (m *VolumeManager) BackupSnapshot(backupName, backupTargetName, volumeName, snapshotName string, labels map[string]string, backupMode string) error {
	if volumeName == "" || snapshotName == "" {
		return fmt.Errorf("volume and snapshot name required")
	}

	if err := m.checkVolumeNotInMigration(volumeName); err != nil {
		return err
	}

	backupCR := &longhorn.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name: backupName,
			Labels: map[string]string{
				types.LonghornLabelBackupTarget: backupTargetName,
			},
		},
		Spec: longhorn.BackupSpec{
			SnapshotName: snapshotName,
			Labels:       labels,
			BackupMode:   longhorn.BackupMode(backupMode),
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

func (m *VolumeManager) GetRunningEngineByVolume(name string) (e *longhorn.Engine, err error) {
	defer func() {
		err = errors.Wrapf(err, "cannot get %v volume engine", name)
	}()

	es, err := m.ds.ListVolumeEngines(name)
	if err != nil {
		return nil, err
	}

	if len(es) == 0 {
		return nil, errors.Errorf("cannot find engine")
	}

	if len(es) != 1 {
		return nil, errors.Errorf("more than one engine exists")
	}

	for _, e = range es {
		break
	}
	if e.Status.CurrentState != longhorn.InstanceStateRunning {
		return nil, errors.Errorf("engine is not running")
	}

	if isReady, err := m.ds.CheckDataEngineImageReadiness(e.Status.CurrentImage, e.Spec.DataEngine, m.currentNodeID); !isReady {
		if err != nil {
			return nil, errors.Errorf("cannot get engine with image %v: %v", e.Status.CurrentImage, err)
		}
		return nil, errors.Errorf("cannot get engine with image %v because it isn't deployed on this node", e.Status.CurrentImage)
	}

	return e, nil
}

func (m *VolumeManager) ListBackupTargetsSorted() ([]*longhorn.BackupTarget, error) {
	backupTargetMap, err := m.ds.ListBackupTargetsRO()
	if err != nil {
		return []*longhorn.BackupTarget{}, err
	}
	backupTargetNames, err := util.SortKeys(backupTargetMap)
	if err != nil {
		return []*longhorn.BackupTarget{}, err
	}
	backupTargets := make([]*longhorn.BackupTarget, len(backupTargetMap))
	for i, backupTargetName := range backupTargetNames {
		backupTargets[i] = backupTargetMap[backupTargetName]
	}
	return backupTargets, nil
}

func (m *VolumeManager) GetBackupTarget(backupTargetName string) (*longhorn.BackupTarget, error) {
	backupTarget, err := m.ds.GetBackupTargetRO(backupTargetName)
	if err != nil {
		return nil, err
	}
	return backupTarget, nil
}

func (m *VolumeManager) CreateBackupTarget(backupTargetName string, backupTargetSpec *longhorn.BackupTargetSpec) (*longhorn.BackupTarget, error) {
	if backupTargetSpec == nil {
		return nil, fmt.Errorf("backup target spec is required")
	}
	backupTarget, err := m.ds.GetBackupTargetRO(backupTargetName)
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			return nil, err
		}

		// Create the default BackupTarget CR if not present
		backupTarget, err = m.ds.CreateBackupTarget(&longhorn.BackupTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: backupTargetName,
			},
			Spec: *backupTargetSpec,
		})
		if err != nil {
			return nil, err
		}
	}
	return backupTarget, nil
}

func (m *VolumeManager) UpdateBackupTarget(backupTargetName string, backupTargetSpec *longhorn.BackupTargetSpec) (*longhorn.BackupTarget, error) {
	if backupTargetSpec == nil {
		return nil, fmt.Errorf("backup target spec is required")
	}
	existingBackupTarget, err := m.ds.GetBackupTarget(backupTargetName)
	if err != nil {
		return nil, err
	}

	if isBackupTargetSpecChanged(backupTargetSpec, &existingBackupTarget.Spec) {
		existingBackupTarget.Spec = *backupTargetSpec.DeepCopy()
		existingBackupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
		existingBackupTarget, err = m.ds.UpdateBackupTarget(existingBackupTarget)
		if err != nil {
			return nil, errors.Wrap(err, "failed to update backup target spec")
		}
	}

	return existingBackupTarget, nil
}

func isBackupTargetSpecChanged(newSpec, existingSpec *longhorn.BackupTargetSpec) bool {
	return newSpec.BackupTargetURL != existingSpec.BackupTargetURL ||
		newSpec.CredentialSecret != existingSpec.CredentialSecret ||
		newSpec.PollInterval != existingSpec.PollInterval
}

func (m *VolumeManager) DeleteBackupTarget(backupTargetName string) error {
	return m.ds.DeleteBackupTarget(backupTargetName)
}

func (m *VolumeManager) SyncBackupTarget(backupTarget *longhorn.BackupTarget) (*longhorn.BackupTarget, error) {
	now := metav1.Time{Time: time.Now().UTC()}
	if now.Sub(backupTarget.Spec.SyncRequestedAt.Time).Seconds() < minSyncBackupTargetIntervalSec {
		return nil, errors.Errorf("cannot synchronize backup target '%v' in %v seconds", backupTarget.Name, minSyncBackupTargetIntervalSec)
	}
	backupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
	return m.ds.UpdateBackupTarget(backupTarget)
}

func (m *VolumeManager) ListBackupVolumes() (map[string]*longhorn.BackupVolume, error) {
	return m.ds.ListBackupVolumes()
}

func (m *VolumeManager) ListBackupVolumesSorted() ([]*longhorn.BackupVolume, error) {
	backupVolumeMap, err := m.ds.ListBackupVolumes()
	if err != nil {
		return []*longhorn.BackupVolume{}, err
	}
	backupVolumeNames, err := util.SortKeys(backupVolumeMap)
	if err != nil {
		return []*longhorn.BackupVolume{}, err
	}
	backupVolumes := make([]*longhorn.BackupVolume, len(backupVolumeMap))
	for i, backupVolumeName := range backupVolumeNames {
		backupVolumes[i] = backupVolumeMap[backupVolumeName]
	}
	return backupVolumes, nil
}

func (m *VolumeManager) GetBackupVolume(backupVolumeName string) (*longhorn.BackupVolume, error) {
	backupVolume, err := m.ds.GetBackupVolumeRO(backupVolumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the BackupVolume CR is not found, return succeeded result
			// This is to compatible with the Longhorn CSI plugin
			// https://github.com/longhorn/longhorn-manager/blob/v1.1.2/csi/controller_server.go#L446-L455
			return &longhorn.BackupVolume{ObjectMeta: metav1.ObjectMeta{Name: backupVolumeName}}, nil
		}
		return nil, err
	}

	return backupVolume, err
}

func (m *VolumeManager) SyncBackupVolume(backupVolume *longhorn.BackupVolume) (*longhorn.BackupVolume, error) {
	now := metav1.Time{Time: time.Now().UTC()}
	if now.Sub(backupVolume.Spec.SyncRequestedAt.Time).Seconds() < minSyncBackupTargetIntervalSec {
		return nil, errors.Errorf("failed to synchronize backup volume '%v' in %v seconds", backupVolume.Name, minSyncBackupTargetIntervalSec)
	}
	backupVolume.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
	return m.ds.UpdateBackupVolume(backupVolume)
}

func (m *VolumeManager) DeleteBackupVolume(backupVolumeName string) error {
	return m.ds.DeleteBackupVolume(backupVolumeName)
}

func (m *VolumeManager) ListAllBackupsSorted() ([]*longhorn.Backup, error) {
	backupMap, err := m.ds.ListBackups()
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := util.SortKeys(backupMap)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backups := make([]*longhorn.Backup, len(backupMap))
	for i, backupName := range backupNames {
		backups[i] = backupMap[backupName]
	}
	return backups, nil
}

func (m *VolumeManager) ListBackupsForVolumeName(volumeName string) (map[string]*longhorn.Backup, error) {
	v, err := m.ds.GetVolumeRO(volumeName)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	backupTargetName := ""
	if v != nil {
		backupTargetName = v.Spec.BackupTargetName
	}
	return m.ds.ListBackupsWithVolumeNameRO(volumeName, backupTargetName)
}

func (m *VolumeManager) ListBackupsForVolumeNameSorted(volumeName string) ([]*longhorn.Backup, error) {
	backupMap, err := m.ListBackupsForVolumeName(volumeName)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := util.SortKeys(backupMap)
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
	v, err := m.ds.GetVolumeRO(volumeName)
	if err != nil {
		return nil, err
	}
	return m.ds.ListBackupsWithBackupTargetAndBackupVolumeRO(v.Spec.BackupTargetName, volumeName)
}

func (m *VolumeManager) ListBackupsForVolumeSorted(volumeName string) ([]*longhorn.Backup, error) {
	backupMap, err := m.ListBackupsForVolume(volumeName)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := util.SortKeys(backupMap)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backups := make([]*longhorn.Backup, len(backupMap))
	for i, backupName := range backupNames {
		backups[i] = backupMap[backupName]
	}
	return backups, nil
}

func (m *VolumeManager) ListBackupsForBackupVolumeSorted(backupVolumeName string) ([]*longhorn.Backup, error) {
	bv, err := m.ds.GetBackupVolumeRO(backupVolumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []*longhorn.Backup{}, nil
		}
		return []*longhorn.Backup{}, err
	}

	backupMap, err := m.ds.ListBackupsWithBackupTargetAndBackupVolumeRO(bv.Spec.BackupTargetName, bv.Spec.VolumeName)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backupNames, err := util.SortKeys(backupMap)
	if err != nil {
		return []*longhorn.Backup{}, err
	}
	backups := make([]*longhorn.Backup, len(backupMap))
	for i, backupName := range backupNames {
		backups[i] = backupMap[backupName]
	}

	return backups, nil
}

func (m *VolumeManager) GetBackup(backupName string) (*longhorn.Backup, error) {
	return m.ds.GetBackupRO(backupName)
}

func (m *VolumeManager) DeleteBackup(backupName string) error {
	return m.ds.DeleteBackup(backupName)
}

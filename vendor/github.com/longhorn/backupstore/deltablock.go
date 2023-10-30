package backupstore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
	"github.com/longhorn/backupstore/util"
)

type DeltaBackupConfig struct {
	BackupName string
	Volume     *Volume
	Snapshot   *Snapshot
	DestURL    string
	DeltaOps   DeltaBlockBackupOperations
	Labels     map[string]string
}

type DeltaRestoreConfig struct {
	BackupURL      string
	DeltaOps       DeltaRestoreOperations
	LastBackupName string
	Filename       string
}

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type BlockInfo struct {
	checksum string
	path     string
	refcount int
}

func isBlockPresent(blk *BlockInfo) bool {
	return blk != nil && blk.path != ""
}

func isBlockReferenced(blk *BlockInfo) bool {
	return blk != nil && blk.refcount > 0
}

func isBlockSafeToDelete(blk *BlockInfo) bool {
	return isBlockPresent(blk) && !isBlockReferenced(blk)
}

type backupRequest struct {
	lastBackup *Backup
}

func (r backupRequest) isIncrementalBackup() bool {
	return r.lastBackup != nil
}

func (r backupRequest) getLastSnapshotName() string {
	if r.lastBackup == nil {
		return ""
	}
	return r.lastBackup.SnapshotName
}

func (r backupRequest) getBackupType() string {
	if r.isIncrementalBackup() {
		return "incremental"
	}
	return "full"
}

type DeltaBlockBackupOperations interface {
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
	UpdateBackupStatus(id, volumeID string, backupProgress int, backupURL string, err string) error
}

type DeltaRestoreOperations interface {
	UpdateRestoreStatus(snapshot string, restoreProgress int, err error)
}

const (
	DEFAULT_BLOCK_SIZE = 2097152

	BLOCKS_DIRECTORY      = "blocks"
	BLOCK_SEPARATE_LAYER1 = 2
	BLOCK_SEPARATE_LAYER2 = 4
	BLK_SUFFIX            = ".blk"

	PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT = 95
	PROGRESS_PERCENTAGE_BACKUP_TOTAL    = 100
)

func CreateDeltaBlockBackup(config *DeltaBackupConfig) (string, bool, error) {
	if config == nil {
		return "", false, fmt.Errorf("invalid empty config for backup")
	}

	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	deltaOps := config.DeltaOps
	if deltaOps == nil {
		return "", false, fmt.Errorf("missing DeltaBlockBackupOperations")
	}

	bsDriver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return "", false, err
	}

	lock, err := New(bsDriver, volume.Name, BACKUP_LOCK)
	if err != nil {
		return "", false, err
	}

	defer lock.Unlock()
	if err := lock.Lock(); err != nil {
		return "", false, err
	}

	if err := addVolume(volume, bsDriver); err != nil {
		return "", false, err
	}

	// Update volume from backupstore
	volume, err = loadVolume(volume.Name, bsDriver)
	if err != nil {
		return "", false, err
	}

	if err := deltaOps.OpenSnapshot(snapshot.Name, volume.Name); err != nil {
		return "", false, err
	}

	backupRequest := &backupRequest{}
	if volume.LastBackupName != "" {
		lastBackupName := volume.LastBackupName
		var backup, err = loadBackup(lastBackupName, volume.Name, bsDriver)
		if err != nil {
			log.WithFields(logrus.Fields{
				LogFieldReason:  LogReasonFallback,
				LogFieldEvent:   LogEventBackup,
				LogFieldObject:  LogObjectBackup,
				LogFieldBackup:  lastBackupName,
				LogFieldVolume:  volume.Name,
				LogFieldDestURL: destURL,
			}).Info("Cannot find previous backup in backupstore")
		} else if backup.SnapshotName == snapshot.Name {
			//Generate full snapshot if the snapshot has been backed up last time
			log.WithFields(logrus.Fields{
				LogFieldReason:   LogReasonFallback,
				LogFieldEvent:    LogEventCompare,
				LogFieldObject:   LogObjectSnapshot,
				LogFieldSnapshot: backup.SnapshotName,
				LogFieldVolume:   volume.Name,
			}).Debug("Create full snapshot config")
		} else if backup.SnapshotName != "" && !deltaOps.HasSnapshot(backup.SnapshotName, volume.Name) {
			log.WithFields(logrus.Fields{
				LogFieldReason:   LogReasonFallback,
				LogFieldObject:   LogObjectSnapshot,
				LogFieldSnapshot: backup.SnapshotName,
				LogFieldVolume:   volume.Name,
			}).Debug("Cannot find last snapshot in local storage")
		} else {
			backupRequest.lastBackup = backup
		}
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:       LogReasonStart,
		LogFieldObject:       LogObjectSnapshot,
		LogFieldEvent:        LogEventCompare,
		LogFieldSnapshot:     snapshot.Name,
		LogFieldLastSnapshot: backupRequest.getLastSnapshotName(),
	}).Debug("Generating snapshot changed blocks config")

	delta, err := deltaOps.CompareSnapshot(snapshot.Name, backupRequest.getLastSnapshotName(), volume.Name)
	if err != nil {
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return "", backupRequest.isIncrementalBackup(), err
	}
	if delta.BlockSize != DEFAULT_BLOCK_SIZE {
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return "", backupRequest.isIncrementalBackup(),
			fmt.Errorf("driver doesn't support block sizes other than %v", DEFAULT_BLOCK_SIZE)
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:       LogReasonComplete,
		LogFieldObject:       LogObjectSnapshot,
		LogFieldEvent:        LogEventCompare,
		LogFieldSnapshot:     snapshot.Name,
		LogFieldLastSnapshot: backupRequest.getLastSnapshotName(),
	}).Debug("Generated snapshot changed blocks config")

	log.WithFields(logrus.Fields{
		LogFieldReason:     LogReasonStart,
		LogFieldEvent:      LogEventBackup,
		LogFieldBackupType: backupRequest.getBackupType(),
		LogFieldSnapshot:   snapshot.Name,
	}).Debug("Creating backup")

	backupName := config.BackupName
	if backupName == "" {
		backupName = util.GenerateName("backup")
	}

	deltaBackup := &Backup{
		Name:         backupName,
		VolumeName:   volume.Name,
		SnapshotName: snapshot.Name,
		Blocks:       []BlockMapping{},
	}

	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return "", backupRequest.isIncrementalBackup(), err
	}
	go func() {
		defer deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		defer lock.Unlock()

		if progress, backup, err := performBackup(config, delta, deltaBackup, backupRequest.lastBackup, bsDriver); err != nil {
			deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, progress, "", err.Error())
		} else {
			deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, progress, backup, "")
		}
	}()
	return deltaBackup.Name, backupRequest.isIncrementalBackup(), nil
}

// performBackup if lastBackup is present we will do an incremental backup
func performBackup(config *DeltaBackupConfig, delta *Mappings, deltaBackup *Backup, lastBackup *Backup,
	bsDriver BackupStoreDriver) (int, string, error) {

	// create an in progress backup config file
	if err := saveBackup(&Backup{Name: deltaBackup.Name, VolumeName: deltaBackup.VolumeName,
		CreatedTime: ""}, bsDriver); err != nil {
		return 0, "", err
	}

	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	deltaOps := config.DeltaOps

	var progress int
	mCounts := len(delta.Mappings)
	newBlocks := int64(0)
	for m, d := range delta.Mappings {
		if d.Size%delta.BlockSize != 0 {
			return progress, "", fmt.Errorf("Mapping's size %v is not multiples of backup block size %v",
				d.Size, delta.BlockSize)
		}
		block := make([]byte, DEFAULT_BLOCK_SIZE)
		blkCounts := d.Size / delta.BlockSize
		for i := int64(0); i < blkCounts; i++ {
			offset := d.Offset + i*delta.BlockSize
			log.Debugf("Backup for %v: segment %v/%v, blocks %v/%v", snapshot.Name, m+1, mCounts, i+1, blkCounts)
			err := deltaOps.ReadSnapshot(snapshot.Name, volume.Name, offset, block)
			if err != nil {
				return progress, "", err
			}
			checksum := util.GetChecksum(block)
			blkFile := getBlockFilePath(volume.Name, checksum)
			if bsDriver.FileExists(blkFile) {
				blockMapping := BlockMapping{
					Offset:        offset,
					BlockChecksum: checksum,
				}
				deltaBackup.Blocks = append(deltaBackup.Blocks, blockMapping)
				log.Debugf("Found existed block match at %v", blkFile)
				continue
			}

			rs, err := util.CompressData(block)
			if err != nil {
				return progress, "", err
			}

			if err := bsDriver.Write(blkFile, rs); err != nil {
				return progress, "", err
			}
			log.Debugf("Created new block file at %v", blkFile)

			newBlocks++
			blockMapping := BlockMapping{
				Offset:        offset,
				BlockChecksum: checksum,
			}
			deltaBackup.Blocks = append(deltaBackup.Blocks, blockMapping)
		}
		progress = int((float64(m+1) / float64(mCounts)) * PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT)
		deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, progress, "", "")
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:   LogReasonComplete,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
	}).Debug("Created snapshot changed blocks")

	backup := mergeSnapshotMap(deltaBackup, lastBackup)
	backup.SnapshotName = snapshot.Name
	backup.SnapshotCreatedAt = snapshot.CreatedTime
	backup.CreatedTime = util.Now()
	backup.Size = int64(len(backup.Blocks)) * DEFAULT_BLOCK_SIZE
	backup.Labels = config.Labels
	backup.IsIncremental = lastBackup != nil

	if err := saveBackup(backup, bsDriver); err != nil {
		return progress, "", err
	}

	volume, err := loadVolume(volume.Name, bsDriver)
	if err != nil {
		return progress, "", err
	}

	volume.LastBackupName = backup.Name
	volume.LastBackupAt = backup.SnapshotCreatedAt
	volume.BlockCount = volume.BlockCount + newBlocks
	// The volume may be expanded
	volume.Size = config.Volume.Size
	volume.Labels = config.Labels
	volume.BackingImageName = config.Volume.BackingImageName
	volume.BackingImageChecksum = config.Volume.BackingImageChecksum

	if err := saveVolume(volume, bsDriver); err != nil {
		return progress, "", err
	}

	return PROGRESS_PERCENTAGE_BACKUP_TOTAL, EncodeBackupURL(backup.Name, volume.Name, destURL), nil
}

func mergeSnapshotMap(deltaBackup, lastBackup *Backup) *Backup {
	if lastBackup == nil {
		return deltaBackup
	}
	backup := &Backup{
		Name:         deltaBackup.Name,
		VolumeName:   deltaBackup.VolumeName,
		SnapshotName: deltaBackup.SnapshotName,
		Blocks:       []BlockMapping{},
	}
	var d, l int
	for d, l = 0, 0; d < len(deltaBackup.Blocks) && l < len(lastBackup.Blocks); {
		dB := deltaBackup.Blocks[d]
		lB := lastBackup.Blocks[l]
		if dB.Offset == lB.Offset {
			backup.Blocks = append(backup.Blocks, dB)
			d++
			l++
		} else if dB.Offset < lB.Offset {
			backup.Blocks = append(backup.Blocks, dB)
			d++
		} else {
			//dB.Offset > lB.offset
			backup.Blocks = append(backup.Blocks, lB)
			l++
		}
	}

	log.WithFields(logrus.Fields{
		LogFieldEvent:      LogEventBackup,
		LogFieldObject:     LogObjectBackup,
		LogFieldBackup:     deltaBackup.Name,
		LogFieldLastBackup: lastBackup.Name,
	}).Debugf("merge backup blocks")
	if d == len(deltaBackup.Blocks) {
		backup.Blocks = append(backup.Blocks, lastBackup.Blocks[l:]...)
	} else {
		backup.Blocks = append(backup.Blocks, deltaBackup.Blocks[d:]...)
	}

	return backup
}

func RestoreDeltaBlockBackup(config *DeltaRestoreConfig) error {
	if config == nil {
		return fmt.Errorf("invalid empty config for restore")
	}

	volDevName := config.Filename
	backupURL := config.BackupURL
	deltaOps := config.DeltaOps
	if deltaOps == nil {
		return fmt.Errorf("missing DeltaRestoreOperations")
	}

	bsDriver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcBackupName, srcVolumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	lock, err := New(bsDriver, srcVolumeName, RESTORE_LOCK)
	if err != nil {
		return err
	}

	defer lock.Unlock()
	if err := lock.Lock(); err != nil {
		return err
	}

	vol, err := loadVolume(srcVolumeName, bsDriver)
	if err != nil {
		return generateError(logrus.Fields{
			LogFieldVolume:    srcVolumeName,
			LogEventBackupURL: backupURL,
		}, "Volume doesn't exist in backupstore: %v", err)
	}

	if vol.Size == 0 || vol.Size%DEFAULT_BLOCK_SIZE != 0 {
		return fmt.Errorf("read invalid volume size %v", vol.Size)
	}

	if _, err := os.Stat(volDevName); err == nil {
		logrus.Warnf("File %s for the restore exists, will remove and re-create it", volDevName)
		if err := os.Remove(volDevName); err != nil {
			return errors.Wrapf(err, "failed to clean up the existing file %v before restore", volDevName)
		}
	}

	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer func() {
		// make sure to close the device
		if err != nil {
			_ = volDev.Close()
		}
	}()

	stat, err := volDev.Stat()
	if err != nil {
		return err
	}

	backup, err := loadBackup(srcBackupName, srcVolumeName, bsDriver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:     LogReasonStart,
		LogFieldEvent:      LogEventRestore,
		LogFieldObject:     LogFieldSnapshot,
		LogFieldSnapshot:   srcBackupName,
		LogFieldOrigVolume: srcVolumeName,
		LogFieldVolumeDev:  volDevName,
		LogEventBackupURL:  backupURL,
	}).Debug()

	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		return err
	}
	go func() {
		defer volDev.Close()
		defer lock.Unlock()

		var progress int
		// This pre-truncate is to ensure the XFS speculatively
		// preallocates post-EOF blocks get reclaimed when volDev is
		// closed.
		// https://github.com/longhorn/longhorn/issues/2503
		// We want to truncate regular files, but not device
		if stat.Mode()&os.ModeType == 0 {
			log.Debugf("Truncate %v to size %v", volDevName, vol.Size)
			if err := volDev.Truncate(vol.Size); err != nil {
				deltaOps.UpdateRestoreStatus(volDevName, progress, err)
				return
			}
		}

		blkCounts := len(backup.Blocks)
		for i, block := range backup.Blocks {
			log.Debugf("Restore for %v: block %v, %v/%v", volDevName, block.BlockChecksum, i+1, blkCounts)
			if err := restoreBlockToFile(srcVolumeName, volDev, bsDriver, block); err != nil {
				deltaOps.UpdateRestoreStatus(volDevName, progress, err)
				return
			}
			progress = int((float64(i+1) / float64(blkCounts)) * PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT)
			deltaOps.UpdateRestoreStatus(volDevName, progress, err)
		}

		deltaOps.UpdateRestoreStatus(volDevName, PROGRESS_PERCENTAGE_BACKUP_TOTAL, nil)
	}()

	return nil
}

func restoreBlockToFile(volumeName string, volDev *os.File, bsDriver BackupStoreDriver, blk BlockMapping) error {
	blkFile := getBlockFilePath(volumeName, blk.BlockChecksum)
	rc, err := bsDriver.Read(blkFile)
	if err != nil {
		return err
	}
	defer rc.Close()
	r, err := util.DecompressAndVerify(rc, blk.BlockChecksum)
	if err != nil {
		return err
	}
	if _, err := volDev.Seek(blk.Offset, 0); err != nil {
		return err
	}
	if _, err := io.CopyN(volDev, r, DEFAULT_BLOCK_SIZE); err != nil {
		return err
	}
	return nil
}

func RestoreDeltaBlockBackupIncrementally(config *DeltaRestoreConfig) error {
	if config == nil {
		return fmt.Errorf("invalid empty config for restore")
	}

	backupURL := config.BackupURL
	volDevName := config.Filename
	lastBackupName := config.LastBackupName
	deltaOps := config.DeltaOps
	if deltaOps == nil {
		return fmt.Errorf("missing DeltaBlockBackupOperations")
	}
	bsDriver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcBackupName, srcVolumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	lock, err := New(bsDriver, srcVolumeName, RESTORE_LOCK)
	if err != nil {
		return err
	}

	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	vol, err := loadVolume(srcVolumeName, bsDriver)
	if err != nil {
		return generateError(logrus.Fields{
			LogFieldVolume:    srcVolumeName,
			LogEventBackupURL: backupURL,
		}, "Volume doesn't exist in backupstore: %v", err)
	}

	if vol.Size == 0 || vol.Size%DEFAULT_BLOCK_SIZE != 0 {
		return fmt.Errorf("read invalid volume size %v", vol.Size)
	}

	// check lastBackupName
	if !util.ValidateName(lastBackupName) {
		return fmt.Errorf("invalid parameter lastBackupName %v", lastBackupName)
	}

	// check the file. do not reuse if the file exists
	if _, err := os.Stat(volDevName); err == nil {
		logrus.Warnf("File %s for the incremental restore exists, will remove and re-create it", volDevName)
		if err := os.Remove(volDevName); err != nil {
			return errors.Wrapf(err, "failed to clean up the existing file %v before incremental restore", volDevName)
		}
	}

	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer func() {
		// make sure to close the device
		if err != nil {
			_ = volDev.Close()
		}
	}()

	stat, err := volDev.Stat()
	if err != nil {
		return err
	}

	lastBackup, err := loadBackup(lastBackupName, srcVolumeName, bsDriver)
	if err != nil {
		return err
	}
	backup, err := loadBackup(srcBackupName, srcVolumeName, bsDriver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:     LogReasonStart,
		LogFieldEvent:      LogEventRestoreIncre,
		LogFieldObject:     LogFieldSnapshot,
		LogFieldSnapshot:   srcBackupName,
		LogFieldOrigVolume: srcVolumeName,
		LogFieldVolumeDev:  volDevName,
		LogEventBackupURL:  backupURL,
	}).Debugf("Started incrementally restoring from %v to %v", lastBackup, backup)
	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		return err
	}
	go func() {
		defer volDev.Close()
		defer lock.Unlock()

		// This pre-truncate is to ensure the XFS speculatively
		// preallocates post-EOF blocks get reclaimed when volDev is
		// closed.
		// https://github.com/longhorn/longhorn/issues/2503
		// We want to truncate regular files, but not device
		if stat.Mode()&os.ModeType == 0 {
			log.Debugf("Truncate %v to size %v", volDevName, vol.Size)
			if err := volDev.Truncate(vol.Size); err != nil {
				deltaOps.UpdateRestoreStatus(volDevName, 0, err)
				return
			}
		}

		if err := performIncrementalRestore(srcVolumeName, volDev, lastBackup, backup, bsDriver, config); err != nil {
			deltaOps.UpdateRestoreStatus(volDevName, 0, err)
			return
		}

		deltaOps.UpdateRestoreStatus(volDevName, PROGRESS_PERCENTAGE_BACKUP_TOTAL, nil)
	}()
	return nil
}

func performIncrementalRestore(srcVolumeName string, volDev *os.File, lastBackup *Backup, backup *Backup,
	bsDriver BackupStoreDriver, config *DeltaRestoreConfig) error {
	var progress int
	volDevName := config.Filename
	deltaOps := config.DeltaOps

	emptyBlock := make([]byte, DEFAULT_BLOCK_SIZE)
	total := len(backup.Blocks) + len(lastBackup.Blocks)

	for b, l := 0, 0; b < len(backup.Blocks) || l < len(lastBackup.Blocks); {
		if b >= len(backup.Blocks) {
			if err := fillBlockToFile(&emptyBlock, volDev, lastBackup.Blocks[l].Offset); err != nil {
				return err
			}
			l++
			continue
		}
		if l >= len(lastBackup.Blocks) {
			if err := restoreBlockToFile(srcVolumeName, volDev, bsDriver, backup.Blocks[b]); err != nil {
				return err
			}
			b++
			continue
		}

		bB := backup.Blocks[b]
		lB := lastBackup.Blocks[l]
		if bB.Offset == lB.Offset {
			if bB.BlockChecksum != lB.BlockChecksum {
				if err := restoreBlockToFile(srcVolumeName, volDev, bsDriver, bB); err != nil {
					return err
				}
			}
			b++
			l++
		} else if bB.Offset < lB.Offset {
			if err := restoreBlockToFile(srcVolumeName, volDev, bsDriver, bB); err != nil {
				return err
			}
			b++
		} else {
			if err := fillBlockToFile(&emptyBlock, volDev, lB.Offset); err != nil {
				return err
			}
			l++
		}
		progress = int((float64(b+l+2) / float64(total)) * PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT)
		deltaOps.UpdateRestoreStatus(volDevName, progress, nil)
	}
	return nil
}

func fillBlockToFile(block *[]byte, volDev *os.File, offset int64) error {
	if _, err := volDev.WriteAt(*block, offset); err != nil {
		return err
	}
	return nil
}

func DeleteBackupVolume(volumeName string, destURL string) error {
	bsDriver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return err
	}
	lock, err := New(bsDriver, volumeName, DELETION_LOCK)
	if err != nil {
		return err
	}

	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	return removeVolume(volumeName, bsDriver)
}

func checkBlockReferenceCount(blockInfos map[string]*BlockInfo, backup *Backup, volumeName string, driver BackupStoreDriver) {
	for _, block := range backup.Blocks {
		info, known := blockInfos[block.BlockChecksum]
		if !known {
			log.Errorf("Backup %v refers to unknown block %v", backup.Name, block.BlockChecksum)
			info = &BlockInfo{checksum: block.BlockChecksum}
			blockInfos[block.BlockChecksum] = info
		}
		info.refcount += 1
	}
}

// getLatestBackup replace lastBackup object if the found
// backup.SnapshotCreatedAt time is greater than the lastBackup
func getLatestBackup(backup *Backup, lastBackup *Backup) error {
	if lastBackup.SnapshotCreatedAt == "" {
		*lastBackup = *backup
		return nil
	}

	backupTime, err := time.Parse(time.RFC3339, backup.SnapshotCreatedAt)
	if err != nil {
		return errors.Wrapf(err, "cannot parse backup %v time %v", backup.Name, backup.SnapshotCreatedAt)
	}

	lastBackupTime, err := time.Parse(time.RFC3339, lastBackup.SnapshotCreatedAt)
	if err != nil {
		return errors.Wrapf(err, "cannot parse last backup %v time %v", lastBackup.Name, lastBackup.SnapshotCreatedAt)
	}

	if backupTime.After(lastBackupTime) {
		*lastBackup = *backup
	}

	return nil
}

func DeleteDeltaBlockBackup(backupURL string) error {
	bsDriver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupName, volumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return err
	}
	log := log.WithFields(logrus.Fields{
		"backup": backupName,
		"volume": volumeName,
	})

	lock, err := New(bsDriver, volumeName, DELETION_LOCK)
	if err != nil {
		return err
	}
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	// If we fail to load the backup we still want to proceed with the deletion of the backup file
	backupToBeDeleted, err := loadBackup(backupName, volumeName, bsDriver)
	if err != nil {
		log.WithError(err).Warn("Failed to load to be deleted backup")
		backupToBeDeleted = &Backup{
			Name:       backupName,
			VolumeName: volumeName,
		}
	}

	// we can delete the requested backupToBeDeleted immediately before GC starts
	if err := removeBackup(backupToBeDeleted, bsDriver); err != nil {
		return err
	}
	log.Info("Removed backup for volume")

	v, err := loadVolume(volumeName, bsDriver)
	if err != nil {
		return errors.Wrap(err, "cannot find volume in backupstore")
	}
	updateLastBackup := false
	if backupToBeDeleted.Name == v.LastBackupName {
		updateLastBackup = true
		v.LastBackupName = ""
		v.LastBackupAt = ""
	}

	log.Debug("GC started")
	deleteBlocks := true
	backupNames, err := getBackupNamesForVolume(volumeName, bsDriver)
	if err != nil {
		log.WithError(err).Warn("Failed to load backup names, skip block deletion")
		deleteBlocks = false
	}

	blockInfos := make(map[string]*BlockInfo)
	blockNames, err := getBlockNamesForVolume(volumeName, bsDriver)
	if err != nil {
		return err
	}
	for _, name := range blockNames {
		blockInfos[name] = &BlockInfo{
			checksum: name,
			path:     getBlockFilePath(volumeName, name),
			refcount: 0,
		}
	}

	lastBackup := &Backup{}
	for _, name := range backupNames {
		log := log.WithField("backup", name)
		backup, err := loadBackup(name, volumeName, bsDriver)
		if err != nil {
			log.WithError(err).Warn("Failed to load backup, skip block deletion")
			deleteBlocks = false
			break
		}

		if isBackupInProgress(backup) {
			log.Info("Found in progress backup, skip block deletion")
			deleteBlocks = false
			break
		}

		// Each volume backup is most likely to reference the same block in the
		// storage target. Reference check single backup metas at a time.
		// https://github.com/longhorn/longhorn/issues/2339
		checkBlockReferenceCount(blockInfos, backup, volumeName, bsDriver)

		if updateLastBackup {
			err := getLatestBackup(backup, lastBackup)
			if err != nil {
				log.WithError(err).Warn("Failed to find last backup, skip block deletion")
				deleteBlocks = false
				break
			}
		}
	}
	if updateLastBackup {
		if deleteBlocks {
			v.LastBackupName = lastBackup.Name
			v.LastBackupAt = lastBackup.SnapshotCreatedAt
		}
		if err := saveVolume(v, bsDriver); err != nil {
			return err
		}
	}

	// check if there have been new backups created while we where processing
	prevBackupNames := backupNames
	backupNames, err = getBackupNamesForVolume(volumeName, bsDriver)
	if err != nil || !util.UnorderedEqual(prevBackupNames, backupNames) {
		log.Info("Found new backups for volume, skip block deletion")
		deleteBlocks = false
	}

	// only delete the blocks if it is safe to do so
	if deleteBlocks {
		if err := cleanupBlocks(blockInfos, volumeName, bsDriver); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBlocks(blockMap map[string]*BlockInfo, volume string, driver BackupStoreDriver) error {
	var deletionFailures []string
	activeBlockCount := int64(0)
	deletedBlockCount := int64(0)
	for _, blk := range blockMap {
		if isBlockSafeToDelete(blk) {
			if err := driver.Remove(blk.path); err != nil {
				deletionFailures = append(deletionFailures, blk.checksum)
				continue
			}
			log.Debugf("Deleted block %v for volume %v", blk.checksum, volume)
			deletedBlockCount++
		} else if isBlockReferenced(blk) && isBlockPresent(blk) {
			activeBlockCount++
		}
	}

	if len(deletionFailures) > 0 {
		return fmt.Errorf("failed to delete backup blocks: %v", deletionFailures)
	}

	log.Debugf("Retained %v blocks for volume %v", activeBlockCount, volume)
	log.Debugf("Removed %v unused blocks for volume %v", deletedBlockCount, volume)
	log.Debug("GC completed")

	v, err := loadVolume(volume, driver)
	if err != nil {
		return err
	}

	// update the block count to what we actually have on disk that is in use
	v.BlockCount = activeBlockCount
	if err := saveVolume(v, driver); err != nil {
		return err
	}
	return nil
}

func getBlockNamesForVolume(volumeName string, driver BackupStoreDriver) ([]string, error) {
	names := []string{}
	blockPathBase := getBlockPath(volumeName)
	lv1Dirs, err := driver.List(blockPathBase)
	// Directory doesn't exist
	if err != nil {
		return names, nil
	}
	for _, lv1 := range lv1Dirs {
		lv1Path := filepath.Join(blockPathBase, lv1)
		lv2Dirs, err := driver.List(lv1Path)
		if err != nil {
			return nil, err
		}
		for _, lv2 := range lv2Dirs {
			lv2Path := filepath.Join(lv1Path, lv2)
			blockNames, err := driver.List(lv2Path)
			if err != nil {
				return nil, err
			}
			names = append(names, blockNames...)
		}
	}

	return util.ExtractNames(names, "", BLK_SUFFIX), nil
}

func getBlockPath(volumeName string) string {
	return filepath.Join(getVolumePath(volumeName), BLOCKS_DIRECTORY) + "/"
}

func getBlockFilePath(volumeName, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlockPath(volumeName), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + BLK_SUFFIX

	return filepath.Join(path, fileName)
}

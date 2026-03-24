package backupstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	. "github.com/longhorn/backupstore/logging"
	"github.com/longhorn/backupstore/types"
	"github.com/longhorn/backupstore/util"
	lhbackup "github.com/longhorn/go-common-libs/backup"
)

type DeltaBackupConfig struct {
	BackupName      string
	Volume          *Volume
	Snapshot        *Snapshot
	DestURL         string
	DeltaOps        DeltaBlockBackupOperations
	Labels          map[string]string
	ConcurrentLimit int32
	Parameters      map[string]string
}

// getBackupBlockSize returns the block size in bytes from the DeltaBackupConfig.
func (config *DeltaBackupConfig) getBackupBlockSize() (int64, error) {
	return getBlockSizeFromParameters(config.Parameters)
}

type DeltaRestoreConfig struct {
	BackupURL       string
	DeltaOps        DeltaRestoreOperations
	LastBackupName  string
	Filename        string
	ConcurrentLimit int32
}

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type Block struct {
	offset            int64
	blockChecksum     string
	compressionMethod string
	isZeroBlock       bool
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

type progress struct {
	sync.Mutex

	totalBlockCounts     int64
	processedBlockCounts int64
	newBlockCounts       int64

	progress int
}

type DeltaBlockBackupOperations interface {
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string, blockSize int64) (*types.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
	UpdateBackupStatus(id, volumeID string, backupState string, backupProgress int, backupURL string, err string) error
}

type DeltaRestoreOperations interface {
	OpenVolumeDev(volDevName string) (*os.File, string, error)
	CloseVolumeDev(volDev *os.File) error
	UpdateRestoreStatus(snapshot string, restoreProgress int, err error)
	Stop()
	GetStopChan() chan struct{}
}

// CreateDeltaBlockBackup creates a delta block backup for the given volume and snapshot.
func CreateDeltaBlockBackup(backupName string, config *DeltaBackupConfig) (isIncremental bool, err error) {
	createLog := log
	defer func() {
		if err != nil {
			createLog.WithError(err).Error("Failed to create delta block backup")
		}
	}()

	if config == nil {
		return false, fmt.Errorf("BUG: invalid empty config for backup")
	}

	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	createLog = createLog.WithFields(logrus.Fields{
		LogFieldVolume:   volume,
		LogFieldSnapshot: snapshot,
		LogFieldDestURL:  destURL,
	})

	deltaOps := config.DeltaOps
	if deltaOps == nil {
		return false, fmt.Errorf("BUG: missing DeltaBlockBackupOperations")
	}

	blockSize, err := config.getBackupBlockSize()
	if err != nil {
		return false, err
	}

	defer func() {
		if err != nil {
			if updateErr := deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(types.ProgressStateError), 0, "", err.Error()); updateErr != nil {
				createLog.WithError(updateErr).Warn("Failed to update backup status")
			}
		}
	}()

	bsDriver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return false, err
	}

	lock, err := New(bsDriver, volume.Name, BACKUP_LOCK)
	if err != nil {
		return false, err
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			createLog.WithError(unlockErr).Warn("Failed to unlock")
		}
	}()
	if err := lock.Lock(); err != nil {
		return false, err
	}

	if err := addVolume(bsDriver, volume); err != nil {
		return false, err
	}

	// Update volume from backupstore
	volume, err = loadVolume(bsDriver, volume.Name)
	if err != nil {
		return false, err
	}

	config.Volume.CompressionMethod = volume.CompressionMethod
	config.Volume.DataEngine = volume.DataEngine
	createLog = createLog.WithFields(logrus.Fields{
		LogFieldCompressionMethod: volume.CompressionMethod,
		LogFieldDataEngine:        volume.DataEngine,
	})

	if err := deltaOps.OpenSnapshot(snapshot.Name, volume.Name); err != nil {
		return false, err
	}

	backupRequest := &backupRequest{}
	if volume.LastBackupName != "" && !isFullBackup(config) {
		lastBackupName := volume.LastBackupName
		createLog = createLog.WithFields(logrus.Fields{
			LogFieldLastBackup: lastBackupName,
		})
		if lastBackup, err := loadBackup(bsDriver, lastBackupName, volume.Name); err != nil {
			createLog = createLog.WithFields(logrus.Fields{
				LogFieldLastBackup: "",
			})
			createLog.WithFields(logrus.Fields{
				LogFieldReason: LogReasonFallback,
				LogFieldEvent:  LogEventBackup,
				LogFieldObject: LogObjectBackup,
			}).WithError(err).Infof("Cannot find previous backup %s in backupstore", lastBackupName)
		} else {
			createLog = createLog.WithFields(logrus.Fields{
				LogFieldLastSnapshot: lastBackup.SnapshotName,
			})
			if lastBackup.SnapshotName == snapshot.Name {
				// Generate full snapshot if the snapshot has been backed up last time
				createLog.WithFields(logrus.Fields{
					LogFieldReason: LogReasonFallback,
					LogFieldEvent:  LogEventCompare,
					LogFieldObject: LogObjectSnapshot,
				}).Info("Creating full snapshot config")
			} else if lastBackup.SnapshotName != "" && !deltaOps.HasSnapshot(lastBackup.SnapshotName, volume.Name) {
				createLog = createLog.WithFields(logrus.Fields{
					LogFieldLastSnapshot: "",
				})
				createLog.WithFields(logrus.Fields{
					LogFieldReason: LogReasonFallback,
					LogFieldObject: LogObjectSnapshot,
				}).Infof("Cannot find last snapshot %s in local storage", lastBackup.SnapshotName)
			} else {
				backupRequest.lastBackup = lastBackup
			}
		}
	}

	createLog = logrus.WithFields(logrus.Fields{
		LogFieldBackupType: backupRequest.getBackupType(),
	})
	createLog.WithFields(logrus.Fields{
		LogFieldReason: LogReasonStart,
		LogFieldObject: LogObjectSnapshot,
		LogFieldEvent:  LogEventCompare,
	}).Info("Generating snapshot changed blocks config")

	delta, err := deltaOps.CompareSnapshot(snapshot.Name, backupRequest.getLastSnapshotName(), volume.Name, blockSize)
	if err != nil {
		if closeErr := deltaOps.CloseSnapshot(snapshot.Name, volume.Name); closeErr != nil {
			err = errors.Wrapf(err, "during handling err %+v, close snapshot returns err %+v", err, closeErr)
		}
		return backupRequest.isIncrementalBackup(), err
	}
	createLog.WithFields(logrus.Fields{
		LogFieldReason: LogReasonComplete,
		LogFieldObject: LogObjectSnapshot,
		LogFieldEvent:  LogEventCompare,
	}).Info("Generated snapshot changed blocks config")

	createLog.WithFields(logrus.Fields{
		LogFieldReason:          LogReasonStart,
		LogFieldEvent:           LogEventBackup,
		LogFieldBackupBlockSize: blockSize,
	}).Info("Creating backup")

	deltaBackup := &Backup{
		Name:              backupName,
		VolumeName:        volume.Name,
		SnapshotName:      snapshot.Name,
		CompressionMethod: volume.CompressionMethod,
		Blocks:            []BlockMapping{},
		ProcessingBlocks: &ProcessingBlocks{
			blocks: map[string][]*BlockMapping{},
		},
	}

	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		if closeErr := deltaOps.CloseSnapshot(snapshot.Name, volume.Name); closeErr != nil {
			err = errors.Wrapf(err, "during handling err %+v, close snapshot returns err %+v", err, closeErr)
		}
		return backupRequest.isIncrementalBackup(), err
	}
	go func() {
		defer func() {
			if closeErr := deltaOps.CloseSnapshot(snapshot.Name, volume.Name); closeErr != nil {
				createLog.WithError(closeErr).Warn("Failed to close snapshot")
			}
		}()
		defer func() {
			if unlockErr := lock.Unlock(); unlockErr != nil {
				createLog.WithError(unlockErr).Warn("Failed to unlock")
			}
		}()

		if updateErr := deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(types.ProgressStateInProgress), 0, "", ""); updateErr != nil {
			createLog.WithError(updateErr).Error("Failed to update backup status")
		}

		createLog.Info("Performing delta block backup")

		if progress, backup, err := performBackup(bsDriver, config, delta, deltaBackup, backupRequest.lastBackup); err != nil {
			createLog.WithError(err).Errorf("Failed to perform backup for volume %v snapshot %v", volume.Name, snapshot.Name)
			if updateErr := deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(types.ProgressStateInProgress), progress, "", err.Error()); updateErr != nil {
				createLog.WithError(updateErr).Warn("Failed to update backup status")
			}
		} else {
			if updateErr := deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(types.ProgressStateInProgress), progress, backup, ""); updateErr != nil {
				createLog.WithError(updateErr).Warn("Failed to update backup status")
			}
		}
	}()
	return backupRequest.isIncrementalBackup(), nil
}

func populateMappings(delta *types.Mappings) (<-chan types.Mapping, <-chan error) {
	mappingChan := make(chan types.Mapping, 1)
	errChan := make(chan error, 1)

	go func() {
		defer close(mappingChan)
		defer close(errChan)

		for _, mapping := range delta.Mappings {
			mappingChan <- mapping
		}
	}()

	return mappingChan, errChan
}

func getProgress(total, processed int64) int {
	return int((float64(processed+1) / float64(total)) * PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT)
}

func isBlockBeingProcessed(deltaBackup *Backup, offset int64, checksum string) bool {
	processingBlocks := deltaBackup.ProcessingBlocks

	processingBlocks.Lock()
	defer processingBlocks.Unlock()

	blockInfo := &BlockMapping{
		Offset:        offset,
		BlockChecksum: checksum,
	}
	if _, ok := processingBlocks.blocks[checksum]; ok {
		processingBlocks.blocks[checksum] = append(processingBlocks.blocks[checksum], blockInfo)
		return true
	}

	processingBlocks.blocks[checksum] = []*BlockMapping{blockInfo}
	return false
}

func updateBlocksAndProgress(deltaBackup *Backup, progress *progress, checksum string, newBlock bool) {
	processingBlocks := deltaBackup.ProcessingBlocks

	processingBlocks.Lock()
	defer processingBlocks.Unlock()

	// Update deltaBackup.Blocks
	blocks := processingBlocks.blocks[checksum]
	for _, block := range blocks {
		deltaBackup.Blocks = append(deltaBackup.Blocks, *block)
	}

	// Update progress
	func() {
		progress.Lock()
		defer progress.Unlock()

		if newBlock {
			progress.newBlockCounts++
		}
		progress.processedBlockCounts += int64(len(blocks))
		progress.progress = getProgress(progress.totalBlockCounts, progress.processedBlockCounts)
	}()

	delete(processingBlocks.blocks, checksum)
}

func backupBlock(bsDriver BackupStoreDriver, config *DeltaBackupConfig,
	deltaBackup *Backup, offset int64, block []byte, progress *progress) error {
	var err error
	newBlock := false
	volume := config.Volume
	snapshot := config.Snapshot
	deltaOps := config.DeltaOps

	checksum := util.GetChecksum(block)

	// This prevents multiple goroutines from trying to upload blocks that contain identical contents
	// with the same checksum but different offsets).
	// After uploading, `bsDriver.FileExists(blkFile)` is used to avoid repeat uploading.
	if isBlockBeingProcessed(deltaBackup, offset, checksum) {
		return nil
	}

	defer func() {
		if err != nil {
			return
		}
		deltaBackup.Lock()
		defer deltaBackup.Unlock()
		updateBlocksAndProgress(deltaBackup, progress, checksum, newBlock)
		if updateErr := deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(types.ProgressStateInProgress), progress.progress, "", ""); updateErr != nil {
			logrus.WithError(updateErr).Warn("Failed to update backup status")
		}
	}()

	blkFile := getBlockFilePath(volume.Name, checksum)
	reUpload := false
	if bsDriver.FileExists(blkFile) {
		if !isFullBackup(config) {
			log.Debugf("Found existing block matching at %v", blkFile)
			return nil
		}
		log.Debugf("Reupload existing block matching at %v", blkFile)
		reUpload = true
	}

	log.Tracef("Uploading block file at %v", blkFile)
	newBlock = !reUpload
	rs, err := util.CompressData(deltaBackup.CompressionMethod, block)
	if err != nil {
		return err
	}

	dataSize, err := getTransferDataSize(rs)
	if err != nil {
		return errors.Wrapf(err, "failed to get transfer data size during saving blocks")
	}

	err = bsDriver.Write(blkFile, rs)
	if err != nil {
		return errors.Wrapf(err, "failed to write data during saving blocks")
	}

	updateUploadDataSize(reUpload, deltaBackup, dataSize)

	return nil
}

func getTransferDataSize(rs io.ReadSeeker) (int64, error) {
	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	// reset to start
	if _, err = rs.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	return size, nil
}

func updateUploadDataSize(reUpload bool, deltaBackup *Backup, dataSize int64) {
	deltaBackup.Lock()
	defer deltaBackup.Unlock()

	if reUpload {
		deltaBackup.ReUploadedDataSize += dataSize
	} else {
		deltaBackup.NewlyUploadedDataSize += dataSize
	}
}

func backupMapping(bsDriver BackupStoreDriver, config *DeltaBackupConfig,
	deltaBackup *Backup, blockSize int64, mapping types.Mapping, progress *progress) error {
	volume := config.Volume
	snapshot := config.Snapshot
	deltaOps := config.DeltaOps

	block := make([]byte, blockSize)
	blkCounts := mapping.Size / blockSize

	for i := int64(0); i < blkCounts; i++ {
		log.Tracef("Backup for %v: segment %+v, blocks %v/%v", snapshot.Name, mapping, i+1, blkCounts)
		offset := mapping.Offset + i*blockSize
		if err := deltaOps.ReadSnapshot(snapshot.Name, volume.Name, offset, block); err != nil {
			logrus.WithError(err).Errorf("Failed to read volume %v snapshot %v block at offset %v size %v",
				volume.Name, snapshot.Name, offset, len(block))
			return err
		}

		if err := backupBlock(bsDriver, config, deltaBackup, offset, block, progress); err != nil {
			logrus.WithError(err).Errorf("Failed to back up volume %v snapshot %v block at offset %v size %v",
				volume.Name, snapshot.Name, offset, len(block))
			return err
		}
	}

	return nil
}

func backupMappings(ctx context.Context, bsDriver BackupStoreDriver, config *DeltaBackupConfig,
	deltaBackup *Backup, blockSize int64, progress *progress, in <-chan types.Mapping) <-chan error {
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		for {
			select {
			case <-ctx.Done():
				return
			case mapping, open := <-in:
				if !open {
					return
				}

				if err := backupMapping(bsDriver, config, deltaBackup, blockSize, mapping, progress); err != nil {
					errChan <- err
					return
				}
			}
		}
	}()

	return errChan
}

func getTotalBackupBlockCounts(delta *types.Mappings) (int64, error) {
	totalBlockCounts := int64(0)
	for _, d := range delta.Mappings {
		if d.Size%delta.BlockSize != 0 {
			return 0, fmt.Errorf("mapping's size %v is not multiples of backup block size %v",
				d.Size, delta.BlockSize)
		}
		totalBlockCounts += d.Size / delta.BlockSize
	}
	return totalBlockCounts, nil
}

func sortBackupBlocks(blocks []BlockMapping, volumeSize, blockSize int64) []BlockMapping {
	sortedBlocks := make([]string, volumeSize/blockSize)
	for _, block := range blocks {
		i := block.Offset / blockSize
		sortedBlocks[i] = block.BlockChecksum
	}

	blockMappings := []BlockMapping{}
	for i, checksum := range sortedBlocks {
		if checksum != "" {
			blockMappings = append(blockMappings, BlockMapping{
				Offset:        int64(i) * blockSize,
				BlockChecksum: checksum,
			})
		}
	}

	return blockMappings
}

// performBackup if lastBackup is present we will do an incremental backup
func performBackup(bsDriver BackupStoreDriver, config *DeltaBackupConfig, delta *types.Mappings, deltaBackup *Backup, lastBackup *Backup) (int, string, error) {
	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	concurrentLimit := config.ConcurrentLimit

	blockSize, err := config.getBackupBlockSize()
	if err != nil {
		logrus.WithError(err).Errorf("Failed to backup volume %v without valid block size", volume.Name)
		return 0, "", err
	}

	// create an in progress backup config file
	if err := saveBackup(bsDriver, &Backup{
		Name:              deltaBackup.Name,
		VolumeName:        deltaBackup.VolumeName,
		CompressionMethod: volume.CompressionMethod,
		CreatedTime:       "",
	}); err != nil {
		return 0, "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	totalBlockCounts, err := getTotalBackupBlockCounts(delta)
	if err != nil {
		return 0, "", err
	}
	logrus.WithField(LogFieldBackupBlockSize, delta.BlockSize).Infof("Volume %v Snapshot %v is consist of %v mappings and %v blocks",
		volume.Name, snapshot.Name, len(delta.Mappings), totalBlockCounts)

	progress := &progress{
		totalBlockCounts: totalBlockCounts,
	}

	mappingChan, errChan := populateMappings(delta)

	errorChans := []<-chan error{errChan}
	for i := 0; i < int(concurrentLimit); i++ {
		errorChans = append(errorChans, backupMappings(ctx, bsDriver, config,
			deltaBackup, delta.BlockSize, progress, mappingChan))
	}

	mergedErrChan := mergeErrorChannels(ctx, errorChans...)
	err = <-mergedErrChan

	if err != nil {
		logrus.WithError(err).Errorf("Failed to backup volume %v snapshot %v", volume.Name, snapshot.Name)
		return progress.progress, "", err
	}

	log.WithFields(logrus.Fields{
		LogFieldReason:          LogReasonComplete,
		LogFieldEvent:           LogEventBackup,
		LogFieldObject:          LogObjectSnapshot,
		LogFieldSnapshot:        snapshot.Name,
		LogFieldBackupBlockSize: delta.BlockSize,
	}).Infof("Created snapshot changed blocks: %v mappings, %v blocks and %v new blocks",
		len(delta.Mappings), progress.totalBlockCounts, progress.newBlockCounts)

	deltaBackup.Blocks = sortBackupBlocks(deltaBackup.Blocks, volume.Size, delta.BlockSize)

	backup := mergeSnapshotMap(deltaBackup, lastBackup)
	backup.SnapshotName = snapshot.Name
	backup.SnapshotCreatedAt = snapshot.CreatedTime
	backup.CreatedTime = util.Now()
	backup.Size = int64(len(backup.Blocks)) * blockSize
	backup.Labels = config.Labels
	backup.Parameters = config.Parameters
	backup.IsIncremental = lastBackup != nil
	backup.NewlyUploadedDataSize = deltaBackup.NewlyUploadedDataSize
	backup.ReUploadedDataSize = deltaBackup.ReUploadedDataSize

	if err := saveBackup(bsDriver, backup); err != nil {
		return progress.progress, "", err
	}

	volume, err = loadVolume(bsDriver, volume.Name)
	if err != nil {
		return progress.progress, "", err
	}

	volume.LastBackupName = backup.Name
	volume.LastBackupAt = backup.SnapshotCreatedAt
	volume.BlockCount = volume.BlockCount + progress.newBlockCounts
	// The volume may be expanded
	volume.Size = config.Volume.Size
	volume.Labels = config.Labels
	volume.BackingImageName = config.Volume.BackingImageName
	volume.BackingImageChecksum = config.Volume.BackingImageChecksum
	volume.CompressionMethod = config.Volume.CompressionMethod
	volume.StorageClassName = config.Volume.StorageClassName
	volume.DataEngine = config.Volume.DataEngine

	if err := saveVolume(bsDriver, volume); err != nil {
		return progress.progress, "", err
	}

	return PROGRESS_PERCENTAGE_BACKUP_TOTAL, EncodeBackupURL(backup.Name, volume.Name, destURL), nil
}

func mergeSnapshotMap(deltaBackup, lastBackup *Backup) *Backup {
	if lastBackup == nil {
		return deltaBackup
	}
	backup := &Backup{
		Name:              deltaBackup.Name,
		VolumeName:        deltaBackup.VolumeName,
		SnapshotName:      deltaBackup.SnapshotName,
		CompressionMethod: deltaBackup.CompressionMethod,
		Blocks:            []BlockMapping{},
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
	}).Info("Merge backup blocks")
	if d == len(deltaBackup.Blocks) {
		backup.Blocks = append(backup.Blocks, lastBackup.Blocks[l:]...)
	} else {
		backup.Blocks = append(backup.Blocks, deltaBackup.Blocks[d:]...)
	}

	return backup
}

// RestoreDeltaBlockBackup restores a delta block backup for the given configuration
func RestoreDeltaBlockBackup(ctx context.Context, config *DeltaRestoreConfig) (err error) {
	restoreLog := log
	defer func() {
		if err != nil {
			restoreLog.WithError(err).Error("Failed to restore delta block backup")
		}
	}()

	if config == nil {
		return fmt.Errorf("invalid empty config for restore")
	}

	volDevName := config.Filename
	backupURL := config.BackupURL
	concurrentLimit := config.ConcurrentLimit
	restoreLog = restoreLog.WithFields(logrus.Fields{
		LogFieldDstVolumeDev:    volDevName,
		LogFieldBackupURL:       backupURL,
		LogFieldConcurrentLimit: concurrentLimit,
	})

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
	restoreLog = restoreLog.WithFields(logrus.Fields{
		LogFieldSnapshot:  srcBackupName,
		LogFieldSrcVolume: srcVolumeName,
	})

	lock, err := New(bsDriver, srcVolumeName, RESTORE_LOCK)
	if err != nil {
		return err
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			restoreLog.WithError(unlockErr).Warn("Failed to unlock")
		}
	}()
	if err := lock.Lock(); err != nil {
		return err
	}

	vol, err := loadVolume(bsDriver, srcVolumeName)
	if err != nil {
		return generateError(logrus.Fields{
			LogFieldSrcVolume: srcVolumeName,
			LogFieldSnapshot:  srcBackupName,
			LogFieldBackupURL: backupURL,
		}, "Source volume doesn't exist in backupstore: %v", err)
	}
	if vol.Size == 0 {
		return fmt.Errorf("invalid volume size %v", vol.Size)
	}
	restoreLog = restoreLog.WithFields(logrus.Fields{
		LogFieldCompressionMethod: vol.CompressionMethod,
		LogFieldDataEngine:        vol.DataEngine,
	})

	volDev, volDevPath, err := deltaOps.OpenVolumeDev(volDevName)
	if err != nil {
		return errors.Wrapf(err, "failed to open volume device %v", volDevName)
	}
	defer func() {
		if err != nil {
			if _err := deltaOps.CloseVolumeDev(volDev); _err != nil {
				restoreLog.WithError(_err).Warnf("Failed to close volume device %v", volDevName)
			}
		}
	}()

	stat, err := volDev.Stat()
	if err != nil {
		return err
	}

	backup, err := loadBackup(bsDriver, srcBackupName, srcVolumeName)
	if err != nil {
		return err
	}

	backupBlockSize, err := backup.GetBlockSize()
	if err != nil {
		return err
	}

	if vol.Size%backupBlockSize != 0 {
		return fmt.Errorf("volume size %v is not a multiple of block size %v", vol.Size, backupBlockSize)
	}
	restoreLog = restoreLog.WithField(LogFieldBackupBlockSize, backupBlockSize)

	restoreLog.WithFields(logrus.Fields{
		LogFieldReason: LogReasonStart,
		LogFieldEvent:  LogEventRestore,
		LogFieldObject: LogFieldSnapshot,
	}).Info("Restoring delta block backup")

	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		return err
	}

	go func(ctx context.Context) {
		var err error
		currentProgress := 0

		defer func() {
			if _err := deltaOps.CloseVolumeDev(volDev); _err != nil {
				restoreLog.WithError(_err).Warnf("Failed to close volume device %v", volDevName)
			}

			deltaOps.UpdateRestoreStatus(volDevName, currentProgress, err)
			if unlockErr := lock.Unlock(); unlockErr != nil {
				restoreLog.WithError(unlockErr).Warn("Failed to unlock")
			}
		}()

		progress := &progress{
			totalBlockCounts: int64(len(backup.Blocks)),
		}

		// This pre-truncate is to ensure the XFS speculatively
		// preallocates post-EOF blocks get reclaimed when volDev is
		// closed.
		// https://github.com/longhorn/longhorn/issues/2503
		// We want to truncate regular files, but not device
		if stat.Mode().IsRegular() {
			restoreLog.Infof("Truncate %v to size %v", volDevName, vol.Size)
			err = volDev.Truncate(vol.Size)
			if err != nil {
				return
			}
		}

		blockChan, errChan := populateBlocksForFullRestore(bsDriver, backup)

		errorChans := []<-chan error{errChan}
		for i := 0; i < int(concurrentLimit); i++ {
			errorChans = append(errorChans, restoreBlocks(ctx, bsDriver, config.DeltaOps, volDevPath, srcVolumeName, blockChan, backupBlockSize, progress))
		}

		mergedErrChan := mergeErrorChannels(ctx, errorChans...)
		err = <-mergedErrChan
		if err != nil {
			currentProgress = progress.progress
			restoreLog.WithError(err).Errorf("Failed to delta restore volume %v backup %v", srcVolumeName, backup.Name)
			return
		}
		currentProgress = PROGRESS_PERCENTAGE_BACKUP_TOTAL
	}(ctx)

	return nil
}

func restoreBlockToFile(bsDriver BackupStoreDriver, volumeName string, volDev *os.File, decompression string, blockSize int64, blk BlockMapping) error {
	blkFile := getBlockFilePath(volumeName, blk.BlockChecksum)
	r, err := DecompressAndVerifyWithFallback(bsDriver, blkFile, decompression, blk.BlockChecksum)
	if err != nil {
		return errors.Wrapf(err, "failed to decompress and verify block %v with checksum %v", blkFile, blk.BlockChecksum)
	}

	if _, err := volDev.Seek(blk.Offset, 0); err != nil {
		return errors.Wrapf(err, "failed to seek to offset %v for decompressed block %v", blk.Offset, blkFile)
	}
	_, err = io.CopyN(volDev, r, blockSize)
	return errors.Wrapf(err, "failed to write decompressed block %v to volume %v", blkFile, volumeName)
}

func RestoreDeltaBlockBackupIncrementally(ctx context.Context, config *DeltaRestoreConfig) (err error) {
	restoreLog := log
	defer func() {
		if err != nil {
			restoreLog.WithError(err).Error("Failed to restore delta block backup incrementally")
		}
	}()

	if config == nil {
		return fmt.Errorf("invalid empty config for restore")
	}

	backupURL := config.BackupURL
	volDevName := config.Filename
	lastBackupName := config.LastBackupName
	restoreLog = restoreLog.WithFields(logrus.Fields{
		LogFieldDstVolumeDev: volDevName,
		LogFieldBackupURL:    backupURL,
		LogFieldLastBackup:   lastBackupName,
	})

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
	restoreLog = restoreLog.WithFields(logrus.Fields{
		LogFieldSnapshot:  srcBackupName,
		LogFieldSrcVolume: srcVolumeName,
	})

	lock, err := New(bsDriver, srcVolumeName, RESTORE_LOCK)
	if err != nil {
		return err
	}

	if err := lock.Lock(); err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			restoreLog.WithError(unlockErr).Warn("Failed to unlock")
		}
	}()

	vol, err := loadVolume(bsDriver, srcVolumeName)
	if err != nil {
		return generateError(logrus.Fields{
			LogFieldVolume:    srcVolumeName,
			LogFieldSnapshot:  srcBackupName,
			LogFieldBackupURL: backupURL,
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
		restoreLog.Warnf("File %s for the incremental restore exists, will remove and re-create it", volDevName)
		if err := os.Remove(volDevName); err != nil {
			return errors.Wrapf(err, "failed to clean up the existing file %v before incremental restore", volDevName)
		}
	}

	volDev, volDevPath, err := deltaOps.OpenVolumeDev(volDevName)
	if err != nil {
		return errors.Wrapf(err, "failed to open volume device %v", volDevName)
	}
	defer func() {
		// make sure to close the device
		if err != nil {
			if _err := deltaOps.CloseVolumeDev(volDev); _err != nil {
				restoreLog.WithError(_err).Warnf("Failed to close volume device %v", volDevName)
			}
		}
	}()

	stat, err := volDev.Stat()
	if err != nil {
		return err
	}

	lastBackup, err := loadBackup(bsDriver, lastBackupName, srcVolumeName)
	if err != nil {
		return err
	}
	backup, err := loadBackup(bsDriver, srcBackupName, srcVolumeName)
	if err != nil {
		return err
	}

	lastBackupBlockSize, err := lastBackup.GetBlockSize()
	if err != nil {
		return err
	}
	backupBlockSize, err := backup.GetBlockSize()
	if err != nil {
		return err
	}

	if vol.Size%backupBlockSize != 0 {
		return fmt.Errorf("volume size %v is not a multiple of block size %v", vol.Size, backupBlockSize)
	} else if backupBlockSize != lastBackupBlockSize {
		return fmt.Errorf("backup block size is changed from %v to %v", lastBackupBlockSize, backupBlockSize)
	}

	restoreLog.WithFields(logrus.Fields{
		LogFieldReason: LogReasonStart,
		LogFieldEvent:  LogEventRestoreIncre,
		LogFieldObject: LogFieldSnapshot,
	}).Infof("Started incrementally restoring from %v to %v", lastBackup, backup)
	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		return err
	}
	go func() {
		var err error
		finalProgress := 0

		defer func() {
			if _err := deltaOps.CloseVolumeDev(volDev); _err != nil {
				restoreLog.WithError(_err).Warnf("Failed to close volume device %v", volDevName)
			}

			deltaOps.UpdateRestoreStatus(volDevName, finalProgress, err)

			if unlockErr := lock.Unlock(); unlockErr != nil {
				restoreLog.WithError(unlockErr).Warn("Failed to unlock")
			}
		}()

		// This pre-truncate is to ensure the XFS speculatively
		// preallocates post-EOF blocks get reclaimed when volDev is
		// closed.
		// https://github.com/longhorn/longhorn/issues/2503
		// We want to truncate regular files, but not device
		if stat.Mode().IsRegular() {
			restoreLog.Infof("Truncate %v to size %v", volDevName, vol.Size)
			err = volDev.Truncate(vol.Size)
			if err != nil {
				return
			}
		}

		err = performIncrementalRestore(ctx, bsDriver, config, srcVolumeName, volDevPath, lastBackup, backup, backupBlockSize)
		if err != nil {
			return
		}

		finalProgress = PROGRESS_PERCENTAGE_BACKUP_TOTAL
	}()
	return nil
}

func populateBlocksForIncrementalRestore(bsDriver BackupStoreDriver, lastBackup, backup *Backup) (<-chan *Block, <-chan error) {
	blockChan := make(chan *Block, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(blockChan)
		defer close(errChan)

		for b, l := 0, 0; b < len(backup.Blocks) || l < len(lastBackup.Blocks); {
			if b >= len(backup.Blocks) {
				blockChan <- &Block{
					offset:      lastBackup.Blocks[l].Offset,
					isZeroBlock: true,
				}
				l++
				continue
			}
			if l >= len(lastBackup.Blocks) {
				blockChan <- &Block{
					offset:            backup.Blocks[b].Offset,
					blockChecksum:     backup.Blocks[b].BlockChecksum,
					compressionMethod: backup.CompressionMethod,
				}
				b++
				continue
			}

			bB := backup.Blocks[b]
			lB := lastBackup.Blocks[l]
			if bB.Offset == lB.Offset {
				if bB.BlockChecksum != lB.BlockChecksum {
					blockChan <- &Block{
						offset:            bB.Offset,
						blockChecksum:     bB.BlockChecksum,
						compressionMethod: backup.CompressionMethod,
					}
				}
				b++
				l++
			} else if bB.Offset < lB.Offset {
				blockChan <- &Block{
					offset:            bB.Offset,
					blockChecksum:     bB.BlockChecksum,
					compressionMethod: backup.CompressionMethod,
				}
				b++
			} else {
				blockChan <- &Block{
					offset:      lB.Offset,
					isZeroBlock: true,
				}
				l++
			}
		}
	}()

	return blockChan, errChan
}

func populateBlocksForFullRestore(bsDriver BackupStoreDriver, backup *Backup) (<-chan *Block, <-chan error) {
	blockChan := make(chan *Block, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(blockChan)
		defer close(errChan)

		for _, block := range backup.Blocks {
			blockChan <- &Block{
				offset:            block.Offset,
				blockChecksum:     block.BlockChecksum,
				compressionMethod: backup.CompressionMethod,
			}
		}
	}()

	return blockChan, errChan
}

func restoreBlock(bsDriver BackupStoreDriver, deltaOps DeltaRestoreOperations, volumeName string, volDev *os.File, block *Block, blockSize int64, progress *progress) error {
	defer func() {
		progress.Lock()
		defer progress.Unlock()

		progress.processedBlockCounts++
		progress.progress = getProgress(progress.totalBlockCounts, progress.processedBlockCounts)
		deltaOps.UpdateRestoreStatus(volumeName, progress.progress, nil)
	}()

	if block.isZeroBlock {
		return fillZeros(volDev, block.offset, blockSize)
	}

	return restoreBlockToFile(bsDriver, volumeName, volDev, block.compressionMethod, blockSize,
		BlockMapping{
			Offset:        block.offset,
			BlockChecksum: block.blockChecksum,
		})
}

func restoreBlocks(ctx context.Context, bsDriver BackupStoreDriver, deltaOps DeltaRestoreOperations, volDevPath, volumeName string, in <-chan *Block, blockSize int64, progress *progress) <-chan error {
	errChan := make(chan error, 1)

	go func() {
		var err error
		defer close(errChan)

		volDev, err := os.OpenFile(volDevPath, os.O_RDWR, 0666)
		if err != nil {
			errChan <- err
			return
		}
		defer func() {
			volDev.Close()
			if err != nil {
				errChan <- err
			}
		}()

		for {
			select {
			case <-ctx.Done():
				err = fmt.Errorf(types.ErrorMsgRestoreCancelled+" since server stop for volume %v", volumeName)
				return
			case <-deltaOps.GetStopChan():
				err = fmt.Errorf(types.ErrorMsgRestoreCancelled+" since received stop signal for volume %v", volumeName)
				return
			case block, open := <-in:
				if !open {
					return
				}

				err = restoreBlock(bsDriver, deltaOps, volumeName, volDev, block, blockSize, progress)
				if err != nil {
					return
				}
			}
		}
	}()

	return errChan
}

// performIncrementalRestore assumes the block sizes are identical between lastBackup and backup.
func performIncrementalRestore(ctx context.Context, bsDriver BackupStoreDriver, config *DeltaRestoreConfig,
	srcVolumeName, volDevPath string, lastBackup *Backup, backup *Backup, blockSize int64) error {
	var err error
	concurrentLimit := config.ConcurrentLimit

	progress := &progress{
		totalBlockCounts: int64(len(backup.Blocks) + len(lastBackup.Blocks)),
	}

	blockChan, errChan := populateBlocksForIncrementalRestore(bsDriver, lastBackup, backup)

	errorChans := []<-chan error{errChan}
	for i := 0; i < int(concurrentLimit); i++ {
		errorChans = append(errorChans, restoreBlocks(ctx, bsDriver, config.DeltaOps, volDevPath, srcVolumeName, blockChan, blockSize, progress))
	}

	mergedErrChan := mergeErrorChannels(ctx, errorChans...)
	err = <-mergedErrChan
	if err != nil {
		logrus.WithError(err).Errorf("Failed to incrementally restore volume %v backup %v", srcVolumeName, backup.Name)
	}

	return err
}

func fillZeros(volDev *os.File, offset, length int64) error {
	return syscall.Fallocate(int(volDev.Fd()), 0, offset, length)
}

func DeleteBackupVolume(volumeName string, destURL string) (err error) {
	deleteLog := log.WithFields(logrus.Fields{
		LogFieldVolume:  volumeName,
		LogFieldDestURL: destURL,
	})
	defer func() {
		if err != nil {
			deleteLog.WithError(err).Errorf("Failed to delete backup volume %v at destination URL %v", volumeName, destURL)
		}
	}()

	bsDriver, err := GetBackupStoreDriver(destURL)
	if err != nil {
		return err
	}

	backupVolumeFolderExists, err := volumeFolderExists(bsDriver, volumeName)
	if err != nil {
		return err
	}

	// No need to lock and remove volume if it does not exist.
	if !backupVolumeFolderExists {
		return nil
	}

	lock, err := New(bsDriver, volumeName, DELETION_LOCK)
	if err != nil {
		return err
	}

	if err := lock.Lock(); err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			deleteLog.WithError(unlockErr).Warn("Failed to unlock")
		}
	}()
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

func copyLastBackupInfo(backup *Backup, lastBackup *LastBackupInfo) {
	lastBackup.Name = backup.Name
	lastBackup.SnapshotCreatedAt = backup.SnapshotCreatedAt
}

// getLatestBackup replace lastBackup object if the found
// backup.SnapshotCreatedAt time is greater than the lastBackup
func getLatestBackup(backup *Backup, lastBackup *LastBackupInfo) error {
	if lastBackup.SnapshotCreatedAt == "" {
		copyLastBackupInfo(backup, lastBackup)
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
		copyLastBackupInfo(backup, lastBackup)
	}

	return nil
}

func DeleteDeltaBlockBackup(backupURL string) (err error) {
	deleteLog := log.WithFields(logrus.Fields{
		LogFieldBackupURL: backupURL,
	})
	defer func() {
		if err != nil {
			log.WithError(err).Error("Failed to delete delta block backup")
		}
	}()

	bsDriver, err := GetBackupStoreDriver(backupURL)
	if err != nil {
		return err
	}

	backupName, volumeName, _, err := DecodeBackupURL(backupURL)
	if err != nil {
		return err
	}
	deleteLog = deleteLog.WithFields(logrus.Fields{
		LogFieldBackup: backupName,
		LogFieldVolume: volumeName,
	})

	lock, err := New(bsDriver, volumeName, DELETION_LOCK)
	if err != nil {
		return err
	}
	if err := lock.Lock(); err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			deleteLog.WithError(unlockErr).Warn("Failed to unlock")
		}
	}()

	// If we fail to load the backup we still want to proceed with the deletion of the backup file
	backupToBeDeleted, err := loadBackup(bsDriver, backupName, volumeName)
	if err != nil {
		deleteLog.WithError(err).Warn("Failed to load to be deleted backup")
		backupToBeDeleted = &Backup{
			Name:       backupName,
			VolumeName: volumeName,
		}
	}

	// we can delete the requested backupToBeDeleted immediately before GC starts
	if err := removeBackup(backupToBeDeleted, bsDriver); err != nil {
		return err
	}
	deleteLog.Info("Removed backup for volume")

	v, err := loadVolume(bsDriver, volumeName)
	if err != nil {
		return errors.Wrap(err, "cannot find volume in backupstore")
	}
	updateLastBackup := false
	if backupToBeDeleted.Name == v.LastBackupName {
		updateLastBackup = true
		v.LastBackupName = ""
		v.LastBackupAt = ""
	}

	deleteLog.Info("GC started")
	deleteBlocks := true
	backupNames, err := getBackupNamesForVolume(bsDriver, volumeName)
	if err != nil {
		deleteLog.WithError(err).Warn("Failed to load backup names, skip block deletion")
		deleteBlocks = false
	}

	blockInfos := make(map[string]*BlockInfo)
	blockNames, err := getBlockNamesForVolume(bsDriver, volumeName)
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

	lastBackup := &LastBackupInfo{}
	for _, name := range backupNames {
		deleteLog = deleteLog.WithField("backup", name)
		backup, err := loadBackup(bsDriver, name, volumeName)
		if err != nil {
			deleteLog.WithError(err).Warn("Failed to load backup, skip block deletion")
			deleteBlocks = false
			break
		}

		if isBackupInProgress(backup) {
			deleteLog.Info("Found in progress backup, skip block deletion")
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
				deleteLog.WithError(err).Warn("Failed to find last backup, skip block deletion")
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
		if err := saveVolume(bsDriver, v); err != nil {
			return err
		}
	}

	// check if there have been new backups created while we where processing
	prevBackupNames := backupNames
	backupNames, err = getBackupNamesForVolume(bsDriver, volumeName)
	if err != nil || !util.UnorderedEqual(prevBackupNames, backupNames) {
		deleteLog.Info("Found new backups for volume, skip block deletion")
		deleteBlocks = false
	}

	// only delete the blocks if it is safe to do so
	if deleteBlocks {
		if err := cleanupBlocks(bsDriver, blockInfos, volumeName); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBlocks(driver BackupStoreDriver, blockMap map[string]*BlockInfo, volume string) error {
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

	log.Infof("Retained %v blocks for volume %v", activeBlockCount, volume)
	log.Infof("Removed %v unused blocks for volume %v", deletedBlockCount, volume)
	log.Info("GC completed")

	v, err := loadVolume(driver, volume)
	if err != nil {
		return err
	}

	// update the block count to what we actually have on disk that is in use
	v.BlockCount = activeBlockCount
	return saveVolume(driver, v)
}

func getBlockNamesForVolume(driver BackupStoreDriver, volumeName string) ([]string, error) {
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

func isFullBackup(config *DeltaBackupConfig) bool {
	if config.Parameters != nil {
		if backupMode, exist := config.Parameters[lhbackup.LonghornBackupParameterBackupMode]; exist {
			return lhbackup.LonghornBackupMode(backupMode) == lhbackup.LonghornBackupModeFull
		}
	}
	return false
}

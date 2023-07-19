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
	"github.com/longhorn/backupstore/util"
)

type DeltaBackupConfig struct {
	BackupName      string
	Volume          *Volume
	Snapshot        *Snapshot
	DestURL         string
	DeltaOps        DeltaBlockBackupOperations
	Labels          map[string]string
	ConcurrentLimit int32
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
	UpdateBackupStatus(id, volumeID string, backupState string, backupProgress int, backupURL string, err string) error
}

type DeltaRestoreOperations interface {
	UpdateRestoreStatus(snapshot string, restoreProgress int, err error)
}

const (
	DEFAULT_BLOCK_SIZE        = 2 * 1024 * 1024
	LEGACY_COMPRESSION_METHOD = "gzip"

	BLOCKS_DIRECTORY      = "blocks"
	BLOCK_SEPARATE_LAYER1 = 2
	BLOCK_SEPARATE_LAYER2 = 4
	BLK_SUFFIX            = ".blk"

	PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT = 95
	PROGRESS_PERCENTAGE_BACKUP_TOTAL    = 100
)

func CreateDeltaBlockBackup(backupName string, config *DeltaBackupConfig) (isIncremental bool, err error) {
	if config == nil {
		return false, fmt.Errorf("BUG: invalid empty config for backup")
	}
	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	deltaOps := config.DeltaOps
	if deltaOps == nil {
		return false, fmt.Errorf("BUG: missing DeltaBlockBackupOperations")
	}

	log := logrus.WithFields(logrus.Fields{
		"volume":   volume,
		"snapshot": snapshot,
		"destURL":  destURL,
	})

	defer func() {
		if err != nil {
			log.WithError(err).Error("Failed to create delta block backup")
			deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(ProgressStateError), 0, "", err.Error())
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

	defer lock.Unlock()
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

	if err := deltaOps.OpenSnapshot(snapshot.Name, volume.Name); err != nil {
		return false, err
	}

	backupRequest := &backupRequest{}
	if volume.LastBackupName != "" {
		lastBackupName := volume.LastBackupName
		var backup, err = loadBackup(bsDriver, lastBackupName, volume.Name)
		if err != nil {
			log.WithFields(logrus.Fields{
				LogFieldReason:  LogReasonFallback,
				LogFieldEvent:   LogEventBackup,
				LogFieldObject:  LogObjectBackup,
				LogFieldBackup:  lastBackupName,
				LogFieldVolume:  volume.Name,
				LogFieldDestURL: destURL,
			}).WithError(err).Info("Cannot find previous backup in backupstore")
		} else if backup.SnapshotName == snapshot.Name {
			// Generate full snapshot if the snapshot has been backed up last time
			log.WithFields(logrus.Fields{
				LogFieldReason:   LogReasonFallback,
				LogFieldEvent:    LogEventCompare,
				LogFieldObject:   LogObjectSnapshot,
				LogFieldSnapshot: backup.SnapshotName,
				LogFieldVolume:   volume.Name,
			}).Info("Creating full snapshot config")
		} else if backup.SnapshotName != "" && !deltaOps.HasSnapshot(backup.SnapshotName, volume.Name) {
			log.WithFields(logrus.Fields{
				LogFieldReason:   LogReasonFallback,
				LogFieldObject:   LogObjectSnapshot,
				LogFieldSnapshot: backup.SnapshotName,
				LogFieldVolume:   volume.Name,
			}).Info("Cannot find last snapshot in local storage")
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
	}).Info("Generating snapshot changed blocks config")

	delta, err := deltaOps.CompareSnapshot(snapshot.Name, backupRequest.getLastSnapshotName(), volume.Name)
	if err != nil {
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return backupRequest.isIncrementalBackup(), err
	}
	if delta.BlockSize != DEFAULT_BLOCK_SIZE {
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return backupRequest.isIncrementalBackup(),
			fmt.Errorf("driver doesn't support block sizes other than %v", DEFAULT_BLOCK_SIZE)
	}
	log.WithFields(logrus.Fields{
		LogFieldReason:       LogReasonComplete,
		LogFieldObject:       LogObjectSnapshot,
		LogFieldEvent:        LogEventCompare,
		LogFieldSnapshot:     snapshot.Name,
		LogFieldLastSnapshot: backupRequest.getLastSnapshotName(),
	}).Info("Generated snapshot changed blocks config")

	log.WithFields(logrus.Fields{
		LogFieldReason:     LogReasonStart,
		LogFieldEvent:      LogEventBackup,
		LogFieldBackupType: backupRequest.getBackupType(),
		LogFieldSnapshot:   snapshot.Name,
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
		deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		return backupRequest.isIncrementalBackup(), err
	}
	go func() {
		defer deltaOps.CloseSnapshot(snapshot.Name, volume.Name)
		defer lock.Unlock()

		deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(ProgressStateInProgress), 0, "", "")

		log.Info("Performing delta block backup")
		if progress, backup, err := performBackup(bsDriver, config, delta, deltaBackup, backupRequest.lastBackup); err != nil {
			logrus.WithError(err).Errorf("Failed to perform backup for volume %v snapshot %v", volume.Name, snapshot.Name)
			deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(ProgressStateInProgress), progress, "", err.Error())
		} else {
			deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(ProgressStateInProgress), progress, backup, "")
		}
	}()
	return backupRequest.isIncrementalBackup(), nil
}

func populateMappings(bsDriver BackupStoreDriver, config *DeltaBackupConfig, deltaBackup *Backup, delta *Mappings) (<-chan Mapping, <-chan error) {
	mappingChan := make(chan Mapping, 1)
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
		deltaOps.UpdateBackupStatus(snapshot.Name, volume.Name, string(ProgressStateInProgress), progress.progress, "", "")
	}()

	blkFile := getBlockFilePath(volume.Name, checksum)
	if bsDriver.FileExists(blkFile) {
		log.Debugf("Found existing block matching at %v", blkFile)
		return nil
	}

	log.Debugf("Creating new block file at %v", blkFile)
	newBlock = true
	rs, err := util.CompressData(deltaBackup.CompressionMethod, block)
	if err != nil {
		return err
	}

	return bsDriver.Write(blkFile, rs)
}

func backupMapping(bsDriver BackupStoreDriver, config *DeltaBackupConfig,
	deltaBackup *Backup, blockSize int64, mapping Mapping, progress *progress) error {
	volume := config.Volume
	snapshot := config.Snapshot
	deltaOps := config.DeltaOps

	block := make([]byte, DEFAULT_BLOCK_SIZE)
	blkCounts := mapping.Size / blockSize

	for i := int64(0); i < blkCounts; i++ {
		log.Debugf("Backup for %v: segment %+v, blocks %v/%v", snapshot.Name, mapping, i+1, blkCounts)
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
	deltaBackup *Backup, blockSize int64, progress *progress, in <-chan Mapping) <-chan error {
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

type progress struct {
	sync.Mutex

	totalBlockCounts     int64
	processedBlockCounts int64
	newBlockCounts       int64

	progress int
}

func getTotalBackupBlockCounts(delta *Mappings) (int64, error) {
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

// mergeErrorChannels will merge all error channels into a single error out channel.
// the error out channel will be closed once the ctx is done or all error channels are closed
// if there is an error on one of the incoming channels the error will be relayed.
func mergeErrorChannels(ctx context.Context, channels ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	wg.Add(len(channels))

	out := make(chan error, len(channels))
	output := func(c <-chan error) {
		defer wg.Done()
		select {
		case err, ok := <-c:
			if ok {
				out <- err
			}
			return
		case <-ctx.Done():
			return
		}
	}

	for _, c := range channels {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
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
func performBackup(bsDriver BackupStoreDriver, config *DeltaBackupConfig, delta *Mappings, deltaBackup *Backup, lastBackup *Backup) (int, string, error) {
	volume := config.Volume
	snapshot := config.Snapshot
	destURL := config.DestURL
	concurrentLimit := config.ConcurrentLimit

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
	logrus.Infof("Volume %v Snapshot %v is consist of %v mappings and %v blocks",
		volume.Name, snapshot.Name, len(delta.Mappings), totalBlockCounts)

	progress := &progress{
		totalBlockCounts: totalBlockCounts,
	}

	mappingChan, errChan := populateMappings(bsDriver, config, deltaBackup, delta)

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
		LogFieldReason:   LogReasonComplete,
		LogFieldEvent:    LogEventBackup,
		LogFieldObject:   LogObjectSnapshot,
		LogFieldSnapshot: snapshot.Name,
	}).Infof("Created snapshot changed blocks: %v mappings, %v blocks and %v new blocks",
		len(delta.Mappings), progress.totalBlockCounts, progress.newBlockCounts)

	deltaBackup.Blocks = sortBackupBlocks(deltaBackup.Blocks, volume.Size, delta.BlockSize)

	backup := mergeSnapshotMap(deltaBackup, lastBackup)
	backup.SnapshotName = snapshot.Name
	backup.SnapshotCreatedAt = snapshot.CreatedTime
	backup.CreatedTime = util.Now()
	backup.Size = int64(len(backup.Blocks)) * DEFAULT_BLOCK_SIZE
	backup.Labels = config.Labels
	backup.IsIncremental = lastBackup != nil

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

func RestoreDeltaBlockBackup(config *DeltaRestoreConfig) error {
	if config == nil {
		return fmt.Errorf("invalid empty config for restore")
	}

	volDevName := config.Filename
	backupURL := config.BackupURL
	concurrentLimit := config.ConcurrentLimit
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

	vol, err := loadVolume(bsDriver, srcVolumeName)
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
		if err := os.RemoveAll(volDevName); err != nil {
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

	backup, err := loadBackup(bsDriver, srcBackupName, srcVolumeName)
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
	}).Info("Restoring delta block backup")

	// keep lock alive for async go routine.
	if err := lock.Lock(); err != nil {
		return err
	}
	go func() {
		defer volDev.Close()
		defer lock.Unlock()

		progress := &progress{
			totalBlockCounts: int64(len(backup.Blocks)),
		}

		// This pre-truncate is to ensure the XFS speculatively
		// preallocates post-EOF blocks get reclaimed when volDev is
		// closed.
		// https://github.com/longhorn/longhorn/issues/2503
		// We want to truncate regular files, but not device
		if stat.Mode()&os.ModeType == 0 {
			log.Debugf("Truncate %v to size %v", volDevName, vol.Size)
			if err := volDev.Truncate(vol.Size); err != nil {
				deltaOps.UpdateRestoreStatus(volDevName, progress.progress, err)
				return
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blockChan, errChan := populateBlocksForFullRestore(bsDriver, backup)

		errorChans := []<-chan error{errChan}
		for i := 0; i < int(concurrentLimit); i++ {
			errorChans = append(errorChans, restoreBlocks(ctx, bsDriver, config, srcVolumeName, blockChan, progress))
		}

		mergedErrChan := mergeErrorChannels(ctx, errorChans...)
		err = <-mergedErrChan
		if err != nil {
			logrus.WithError(err).Errorf("Failed to delta restore volume %v backup %v", srcVolumeName, backup.Name)
			deltaOps.UpdateRestoreStatus(volDevName, progress.progress, err)
			return
		}

		deltaOps.UpdateRestoreStatus(volDevName, PROGRESS_PERCENTAGE_BACKUP_TOTAL, nil)
	}()

	return nil
}

func restoreBlockToFile(bsDriver BackupStoreDriver, volumeName string, volDev *os.File, decompression string, blk BlockMapping) error {
	blkFile := getBlockFilePath(volumeName, blk.BlockChecksum)
	rc, err := bsDriver.Read(blkFile)
	if err != nil {
		return err
	}
	defer rc.Close()
	r, err := util.DecompressAndVerify(decompression, rc, blk.BlockChecksum)
	if err != nil {
		return err
	}
	if _, err := volDev.Seek(blk.Offset, 0); err != nil {
		return err
	}
	_, err = io.CopyN(volDev, r, DEFAULT_BLOCK_SIZE)
	return err
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

	vol, err := loadVolume(bsDriver, srcVolumeName)
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

	lastBackup, err := loadBackup(bsDriver, lastBackupName, srcVolumeName)
	if err != nil {
		return err
	}
	backup, err := loadBackup(bsDriver, srcBackupName, srcVolumeName)
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
	}).Infof("Started incrementally restoring from %v to %v", lastBackup, backup)
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

		if err := performIncrementalRestore(bsDriver, config, srcVolumeName, volDevName, lastBackup, backup); err != nil {
			deltaOps.UpdateRestoreStatus(volDevName, 0, err)
			return
		}

		deltaOps.UpdateRestoreStatus(volDevName, PROGRESS_PERCENTAGE_BACKUP_TOTAL, nil)
	}()
	return nil
}

type Block struct {
	offset            int64
	blockChecksum     string
	compressionMethod string
	isZeroBlock       bool
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

func restoreBlock(bsDriver BackupStoreDriver, config *DeltaRestoreConfig,
	volumeName string, volDev *os.File, block *Block, progress *progress) error {
	deltaOps := config.DeltaOps

	defer func() {
		progress.Lock()
		defer progress.Unlock()

		progress.processedBlockCounts++
		progress.progress = getProgress(progress.totalBlockCounts, progress.processedBlockCounts)
		deltaOps.UpdateRestoreStatus(volumeName, progress.progress, nil)
	}()

	if block.isZeroBlock {
		return fillZeros(volDev, block.offset, DEFAULT_BLOCK_SIZE)
	}

	return restoreBlockToFile(bsDriver, volumeName, volDev, block.compressionMethod,
		BlockMapping{
			Offset:        block.offset,
			BlockChecksum: block.blockChecksum,
		})
}

func restoreBlocks(ctx context.Context, bsDriver BackupStoreDriver, config *DeltaRestoreConfig,
	volumeName string, in <-chan *Block, progress *progress) <-chan error {
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)

		volDevName := config.Filename
		volDev, err := os.OpenFile(volDevName, os.O_RDWR, 0666)
		if err != nil {
			errChan <- err
			return
		}
		defer volDev.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case block, open := <-in:
				if !open {
					return
				}

				if err := restoreBlock(bsDriver, config, volumeName, volDev, block, progress); err != nil {
					errChan <- err
					return
				}
			}
		}
	}()

	return errChan
}

func performIncrementalRestore(bsDriver BackupStoreDriver, config *DeltaRestoreConfig,
	srcVolumeName, volDevName string, lastBackup *Backup, backup *Backup) error {
	var err error
	concurrentLimit := config.ConcurrentLimit

	progress := &progress{
		totalBlockCounts: int64(len(backup.Blocks) + len(lastBackup.Blocks)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockChan, errChan := populateBlocksForIncrementalRestore(bsDriver, lastBackup, backup)

	errorChans := []<-chan error{errChan}
	for i := 0; i < int(concurrentLimit); i++ {
		errorChans = append(errorChans, restoreBlocks(ctx, bsDriver, config, srcVolumeName, blockChan, progress))
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
	backupToBeDeleted, err := loadBackup(bsDriver, backupName, volumeName)
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

	log.Info("GC started")
	deleteBlocks := true
	backupNames, err := getBackupNamesForVolume(bsDriver, volumeName)
	if err != nil {
		log.WithError(err).Warn("Failed to load backup names, skip block deletion")
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

	lastBackup := &Backup{}
	for _, name := range backupNames {
		log := log.WithField("backup", name)
		backup, err := loadBackup(bsDriver, name, volumeName)
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
		if err := saveVolume(bsDriver, v); err != nil {
			return err
		}
	}

	// check if there have been new backups created while we where processing
	prevBackupNames := backupNames
	backupNames, err = getBackupNamesForVolume(bsDriver, volumeName)
	if err != nil || !util.UnorderedEqual(prevBackupNames, backupNames) {
		log.Info("Found new backups for volume, skip block deletion")
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

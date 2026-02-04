package common

import (
	"context"
	"sync"

	"github.com/longhorn/backupstore"
)

type ProgressState string

const (
	ProgressStateInProgress = ProgressState("in_progress")
	ProgressStateComplete   = ProgressState("complete")
	ProgressStateError      = ProgressState("error")
)

const (
	ProgressPercentageBackup      = 95
	ProgressPercentageBackupTotal = 100
)

type Mapping struct {
	Offset int64
	Size   int64
}

type Mappings struct {
	Mappings  []Mapping
	BlockSize int64
}

type MessageType string

const (
	MessageTypeError = MessageType("error")
)

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type BlockInfo struct {
	Checksum string
	Path     string
	Refcount int
}

type Block struct {
	Offset            int64
	BlockChecksum     string
	CompressionMethod string
	IsZeroBlock       bool
}

type ProcessingBlocks struct {
	sync.Mutex
	Blocks map[string][]*BlockMapping
}

type Progress struct {
	sync.Mutex

	TotalBlockCounts     int64
	ProcessedBlockCounts int64
	NewBlockCounts       int64

	Progress int
}

func PopulateMappings(bsDriver backupstore.BackupStoreDriver, mappings *Mappings) (<-chan Mapping, <-chan error) {
	mappingChan := make(chan Mapping, 1)
	errChan := make(chan error, 1)

	go func() {
		defer close(mappingChan)
		defer close(errChan)

		for _, mapping := range mappings.Mappings {
			mappingChan <- mapping
		}
	}()

	return mappingChan, errChan
}

func PopulateBlocksForFullRestore(blocks []BlockMapping, compressionMethod string) (<-chan *Block, <-chan error) {
	blockChan := make(chan *Block, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(blockChan)
		defer close(errChan)

		for _, block := range blocks {
			blockChan <- &Block{
				Offset:            block.Offset,
				BlockChecksum:     block.BlockChecksum,
				CompressionMethod: compressionMethod,
			}
		}
	}()

	return blockChan, errChan
}

// MergeErrorChannels will merge all error channels into a single error out channel.
// the error out channel will be closed once the ctx is done or all error channels are closed
// if there is an error on one of the incoming channels the error will be relayed.
func MergeErrorChannels(ctx context.Context, channels ...<-chan error) <-chan error {
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

func GetProgress(total, processed int64) int {
	return int((float64(processed+1) / float64(total)) * ProgressPercentageBackup)
}

func SortBackupBlocks(blocks []BlockMapping, size, blockSize int64) []BlockMapping {
	blocksNum := size / blockSize
	if size%blockSize > 0 {
		blocksNum++
	}
	sortedBlocks := make([]string, blocksNum)
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

func UpdateBlockReferenceCount(blockInfos map[string]*BlockInfo, blocks []BlockMapping, driver backupstore.BackupStoreDriver) {
	for _, block := range blocks {
		info, known := blockInfos[block.BlockChecksum]
		if !known {
			info = &BlockInfo{Checksum: block.BlockChecksum}
			blockInfos[block.BlockChecksum] = info
		}
		info.Refcount++
	}
}

func IsBlockSafeToDelete(blk *BlockInfo) bool {
	return isBlockPresent(blk) && !isBlockReferenced(blk)
}

func isBlockPresent(blk *BlockInfo) bool {
	return blk != nil && blk.Path != ""
}

func isBlockReferenced(blk *BlockInfo) bool {
	return blk != nil && blk.Refcount > 0
}

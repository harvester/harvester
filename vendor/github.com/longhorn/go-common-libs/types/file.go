package types

import (
	"time"
)

var FileLockDefaultTimeout = 24 * time.Hour

type DiskStat struct {
	DiskID           string
	Path             string
	Type             string
	FreeBlocks       int64
	TotalBlocks      int64
	BlockSize        int64
	StorageMaximum   int64
	StorageAvailable int64
}

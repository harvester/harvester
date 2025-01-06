package types

import (
	"time"
)

var FileLockDefaultTimeout = 24 * time.Hour

type DiskDriver string

const (
	DiskDriverNone          = DiskDriver("")
	DiskDriverAuto          = DiskDriver("auto")
	DiskDriverAio           = DiskDriver("aio")
	DiskDriverNvme          = DiskDriver("nvme")
	DiskDriverVirtioScsi    = DiskDriver("virtio-scsi")
	DiskDriverVirtioBlk     = DiskDriver("virtio-blk")
	DiskDriverVirtioPci     = DiskDriver("virtio-pci")
	DiskDriverUioPciGeneric = DiskDriver("uio_pci_generic")
	DiskDriverVfioPci       = DiskDriver("vfio_pci")
)

type DiskStat struct {
	DiskID           string
	Name             string
	Path             string
	Type             string
	Driver           DiskDriver
	FreeBlocks       int64
	TotalBlocks      int64
	BlockSize        int64
	StorageMaximum   int64
	StorageAvailable int64
}

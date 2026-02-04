package api

type DiskInfo struct {
	ID          string
	Name        string
	UUID        string
	Path        string
	Type        string
	Driver      string
	TotalSize   int64
	FreeSize    int64
	TotalBlocks int64
	FreeBlocks  int64
	BlockSize   int64
	ClusterSize int64
}

// ReplicaStorageInstance is utilized to represent a replica directory of a legacy volume and
// a replica logical volume (lvol) of a SPDK volume.
type ReplicaStorageInstance struct {
	Name       string
	UUID       string
	DiskName   string
	DiskUUID   string
	SpecSize   uint64
	ActualSize uint64
}

package client

type SnapshotCloneStatus struct {
	IsCloning          bool
	Error              string
	Progress           int
	State              string
	FromReplicaAddress string
	SnapshotName       string
}

type SnapshotPurgeStatus struct {
	Error     string
	IsPurging bool
	Progress  int
	State     string
}

type SnapshotBackupStatus struct {
	Progress       int
	BackupURL      string
	Error          string
	SnapshotName   string
	State          string
	ReplicaAddress string
}

type BackupRestoreStatus struct {
	IsRestoring            bool
	LastRestored           string
	CurrentRestoringBackup string
	Progress               int
	Error                  string
	Filename               string
	State                  string
	BackupURL              string
}

type EngineBackupVolumeInfo struct {
	Name                 string
	Size                 int64
	Labels               map[string]string
	Created              string
	LastBackupName       string
	LastBackupAt         string
	DataStored           int64
	Messages             map[string]string
	Backups              map[string]*EngineBackupInfo
	BackingImageName     string
	BackingImageChecksum string
}

type EngineBackupInfo struct {
	Name                   string
	URL                    string
	SnapshotName           string
	SnapshotCreated        string
	Created                string
	Size                   int64
	Labels                 map[string]string
	IsIncremental          bool
	VolumeName             string
	VolumeSize             int64
	VolumeCreated          string
	VolumeBackingImageName string
	Messages               map[string]string
}

type ReplicaRebuildStatus struct {
	Error                 string
	IsRebuilding          bool
	Progress              int
	State                 string
	FromReplicaAddress    string
	AppliedRebuildingMBps int64
}

type SnapshotHashStatus struct {
	State             string
	Checksum          string
	Error             string
	SilentlyCorrupted bool
}

type Metrics struct {
	ReadThroughput  uint64
	WriteThroughput uint64
	ReadLatency     uint64
	WriteLatency    uint64
	ReadIOPS        uint64
	WriteIOPS       uint64
}

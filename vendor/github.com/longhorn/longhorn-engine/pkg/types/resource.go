package types

type ReplicaInfo struct {
	Dirty                     bool                `json:"dirty"`
	Rebuilding                bool                `json:"rebuilding"`
	Head                      string              `json:"head"`
	Parent                    string              `json:"parent"`
	Size                      string              `json:"size"`
	SectorSize                int64               `json:"sectorSize,string"`
	BackingFile               string              `json:"backingFile"`
	State                     string              `json:"state"`
	Chain                     []string            `json:"chain"`
	Disks                     map[string]DiskInfo `json:"disks"`
	RemainSnapshots           int                 `json:"remainsnapshots"`
	RevisionCounter           int64               `json:"revisioncounter,string"`
	LastModifyTime            int64               `json:"lastmodifytime"`
	HeadFileSize              int64               `json:"headfilesize"`
	RevisionCounterDisabled   bool                `json:"revisioncounterdisabled"`
	UnmapMarkDiskChainRemoved bool                `json:"unmapMarkDiskChainRemoved"`
	SnapshotCountUsage        int                 `json:"snapshotCountUsage"`
	SnapshotSizeUsage         int64               `json:"snapshotSizeUsage"`
}

type DiskInfo struct {
	Name        string            `json:"name"`
	Parent      string            `json:"parent"`
	Children    map[string]bool   `json:"children"`
	Removed     bool              `json:"removed"`
	UserCreated bool              `json:"usercreated"`
	Created     string            `json:"created"`
	Size        string            `json:"size"`
	Labels      map[string]string `json:"labels"`
}

type PrepareRemoveAction struct {
	Action string `json:"action"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type VolumeInfo struct {
	Name                      string `json:"name"`
	Size                      int64  `json:"size"`
	ReplicaCount              int    `json:"replicaCount"`
	Endpoint                  string `json:"endpoint"`
	Frontend                  string `json:"frontend"`
	FrontendState             string `json:"frontendState"`
	IsExpanding               bool   `json:"isExpanding"`
	LastExpansionError        string `json:"lastExpansionError"`
	LastExpansionFailedAt     string `json:"lastExpansionFailedAt"`
	UnmapMarkSnapChainRemoved bool   `json:"unmapMarkSnapChainRemoved"`
	SnapshotMaxCount          int    `json:"snapshotMaxCount"`
	SnapshotMaxSize           int64  `json:"SnapshotMaxSize"`
}

type ControllerReplicaInfo struct {
	Address string `json:"address"`
	Mode    Mode   `json:"mode"`
}

type SyncFileInfo struct {
	FromFileName string `json:"fromFileName"`
	ToFileName   string `json:"toFileName"`
	ActualSize   int64  `json:"actualSize"`
}

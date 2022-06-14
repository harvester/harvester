package volume

type ExportVolumeInput struct {
	DisplayName      string `json:"displayName"`
	Namespace        string `json:"namespace"`
	StorageClassName string `json:"storageClassName"`
}

type CloneVolumeInput struct {
	Name string `json:"name"`
}

type SnapshotVolumeInput struct {
	Name string `json:"name"`
}

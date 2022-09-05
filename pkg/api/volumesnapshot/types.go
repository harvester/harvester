package volumesnapshot

type RestoreSnapshotInput struct {
	Name             string `json:"name"`
	StorageClassName string `json:"storageClassName"`
}

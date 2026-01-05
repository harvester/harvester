package driver

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	// AnnotationPVCRestoreRef is used to track the PVCRestore that created a resource
	AnnotationPVCRestoreRef = "harvesterhci.io/pvcrestore-ref"
)

type BackupOperation interface {
	Create(vrb *harvesterv1.VolumeRemoteBackup) error
	Readiness(vrb *harvesterv1.VolumeRemoteBackup) (bool, error)
	Delete(vrb *harvesterv1.VolumeRemoteBackup) error
}

type RestoreOperation interface {
	Create(vrr *harvesterv1.VolumeRemoteRestore) error
	Readiness(vrr *harvesterv1.VolumeRemoteRestore) (bool, error)
	Delete(vrr *harvesterv1.VolumeRemoteRestore) error
}

package driver

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	// AnnotationPVCRestoreRef is used to track the PVCRestore that created a resource
	AnnotationPVCRestoreRef = "harvesterhci.io/pvcrestore-ref"
)

type BackupOperation interface {
	Create(pb *harvesterv1.PVCBackup) error
	Readiness(pb *harvesterv1.PVCBackup) (bool, error)
	Delete(pb *harvesterv1.PVCBackup) error
}

type RestoreOperation interface {
	Create(pr *harvesterv1.PVCRestore) error
	Readiness(pr *harvesterv1.PVCRestore) (bool, error)
	Delete(pr *harvesterv1.PVCRestore) error
}

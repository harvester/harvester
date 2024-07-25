package util

// Use util/volume.go for volume related utility functions that would otherwise belong in controller/utils.go except
// that they are used by other components as well (e.g. manager).

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func IsVolumeMigrating(v *longhorn.Volume) bool {
	return v.Spec.MigrationNodeID != "" || v.Status.CurrentMigrationNodeID != ""
}

func IsMigratableVolume(v *longhorn.Volume) bool {
	return v.Spec.Migratable && v.Spec.AccessMode == longhorn.AccessModeReadWriteMany
}

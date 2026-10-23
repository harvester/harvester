package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	harvesterutil "github.com/harvester/harvester/pkg/util"
)

func TestValidateProvisionerAndConfigSnapshotClass(t *testing.T) {
	const driver = "driver.example.io"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				harvesterutil.AnnStorageProvisioner: driver,
			},
		},
	}
	tests := []struct {
		name       string
		backupType harvesterv1.BackupType
		driverInfo settings.CSIDriverInfo
		wantError  bool
	}{
		{
			name:       "native backup uses backup class",
			backupType: harvesterv1.Backup,
			driverInfo: settings.CSIDriverInfo{BackupVolumeSnapshotClassName: "remote-backup"},
		},
		{
			name:       "snapshot uses local class",
			backupType: harvesterv1.Snapshot,
			driverInfo: settings.CSIDriverInfo{VolumeSnapshotClassName: "local-snapshot"},
		},
		{
			name:       "restic uses local class",
			backupType: harvesterv1.Restic,
			driverInfo: settings.CSIDriverInfo{VolumeSnapshotClassName: "local-snapshot"},
		},
		{
			name:       "kopia uses local class",
			backupType: harvesterv1.Kopia,
			driverInfo: settings.CSIDriverInfo{VolumeSnapshotClassName: "local-snapshot"},
		},
		{
			name:       "restic rejects backup class only",
			backupType: harvesterv1.Restic,
			driverInfo: settings.CSIDriverInfo{BackupVolumeSnapshotClassName: "remote-backup"},
			wantError:  true,
		},
		{
			name:       "native backup rejects local class only",
			backupType: harvesterv1.Backup,
			driverInfo: settings.CSIDriverInfo{VolumeSnapshotClassName: "local-snapshot"},
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProvisionerAndConfig(
				pvc,
				nil,
				tt.backupType,
				map[string]settings.CSIDriverInfo{driver: tt.driverInfo},
			)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

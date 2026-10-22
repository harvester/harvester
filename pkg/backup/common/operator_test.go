package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func TestGetVSCName(t *testing.T) {
	driverInfo := settings.CSIDriverInfo{
		VolumeSnapshotClassName:       "local-snapshot",
		BackupVolumeSnapshotClassName: "remote-backup",
	}
	tests := []struct {
		name       string
		backupType harvesterv1.BackupType
		want       string
		wantError  bool
	}{
		{name: "native backup", backupType: harvesterv1.Backup, want: "remote-backup"},
		{name: "snapshot", backupType: harvesterv1.Snapshot, want: "local-snapshot"},
		{name: "restic", backupType: harvesterv1.Restic, want: "local-snapshot"},
		{name: "kopia", backupType: harvesterv1.Kopia, want: "local-snapshot"},
		{name: "unsupported", backupType: "unsupported", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := getVSCName(tt.backupType, driverInfo, "driver.example.io")
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, name)
		})
	}
}

func TestBuildVolumeBackupUsesSafeName(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{}
	pv := &corev1.PersistentVolume{Spec: corev1.PersistentVolumeSpec{
		PersistentVolumeSource: corev1.PersistentVolumeSource{
			CSI: &corev1.CSIPersistentVolumeSource{Driver: "driver.example.io"},
		},
	}}

	short := buildVolumeBackup("backup", "disk", "pvc", pvc, pv, 1)
	require.Equal(t, "backup-volume-pvc", *short.Name)

	longName := strings.Repeat("backup", 20)
	first := buildVolumeBackup(longName, "disk", "pvc-a", pvc, pv, 1)
	second := buildVolumeBackup(longName, "disk", "pvc-b", pvc, pv, 1)
	require.LessOrEqual(t, len(*first.Name), 63)
	require.NotEqual(t, *first.Name, *second.Name)
}

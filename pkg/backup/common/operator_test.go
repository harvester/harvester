package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

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

func TestSanitizeVolBackupsPreservesCheckpoint(t *testing.T) {
	vmb := &harvesterv1.VirtualMachineBackup{
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{
				Name: ptr.To("disk"), ReadyToUse: ptr.To(true),
				CreationTime: ptr.To(metav1.Now()),
				Error:        &harvesterv1.Error{Message: ptr.To("previous error")},
			}},
		},
	}
	before := vmb.DeepCopy()
	volumes := NewVMBackupReader().GetSanitizeVolBackups(vmb)
	require.Equal(t, before, vmb)
	require.Nil(t, volumes[0].ReadyToUse)
	require.Nil(t, volumes[0].CreationTime)
	require.Nil(t, volumes[0].Error)
	*volumes[0].Name = "changed"
	require.Equal(t, before, vmb, "exported metadata must not alias live status")
}

package common

import (
	"testing"

	"github.com/stretchr/testify/require"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func TestGetCleanupTarget(t *testing.T) {
	vmbr := NewVMBackupReader()
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	for _, scenario := range []string{"missing", "disconnected", "connected", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			vmb := &harvesterv1.VirtualMachineBackup{}
			vmb.Status.BackupTarget = &harvesterv1.BackupTarget{Type: "s3", BucketName: "old"}
			value := `{"type":"s3","bucketName":"new"}`
			switch scenario {
			case "missing":
				vmb.Status.BackupTarget = nil
				value = "invalid"
			case "disconnected":
				value = settings.BackupTargetSet.GetDefault()
			case "invalid":
				value = "invalid"
			}
			require.NoError(t, settings.BackupTargetSet.Set(value))
			target, err := GetCleanupTarget(vmbr, vmb)
			if scenario == "invalid" {
				require.ErrorContains(t, err, "failed to decode backup target")
				return
			}
			require.NoError(t, err)
			if scenario == "connected" {
				require.Equal(t, "new", target.BucketName)
				return
			}
			require.Nil(t, target)
		})
	}
}

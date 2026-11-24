package backup

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
)

func TestDataMoverRestoreProgress(t *testing.T) {
	for _, kind := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia} {
		for _, tc := range []struct {
			name     string
			sizes    []int64
			progress []int64
			want     int
		}{
			{name: "single volume", sizes: []int64{10 << 30}, progress: []int64{4}, want: 4},
			{name: "weighted volumes", sizes: []int64{10 << 30, 30 << 30}, progress: []int64{20, 60}, want: 50},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				h, vmb, client := restoreProgressFixture(kind, tc.sizes, tc.progress)
				require.NoError(t, h.vmro.InitVolumesStatus(client.current, vmb))
				require.Len(t, client.current.Status.VolumeRestores, len(tc.sizes))
				for i, vr := range client.current.Status.VolumeRestores {
					require.Equal(t, tc.sizes[i], vr.VolumeSize)
				}

				updated := client.current.DeepCopy()
				require.NoError(t, h.updateProgressMetrics(updated, vmb))
				require.NoError(t, h.saveVolumeProgress(client.current, updated))
				require.Equal(t, tc.want, client.current.Status.Progress)
				for i, vr := range client.current.Status.VolumeRestores {
					require.Equal(t, int(tc.progress[i]), vr.Progress)
				}
				require.False(t, h.vmro.IsComplete(client.current))
			})
		}
	}
}

func restoreProgressFixture(
	kind harvesterv1.BackupType, sizes, progress []int64,
) (*RestoreHandler, *harvesterv1.VirtualMachineBackup, *retryRestoreClient) {
	client := &retryRestoreClient{current: &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
		Spec:       harvesterv1.VirtualMachineRestoreSpec{NewVM: true, VirtualMachineBackupName: "backup"},
	}}
	vmb := &harvesterv1.VirtualMachineBackup{
		Spec: harvesterv1.VirtualMachineBackupSpec{Type: kind},
	}
	re := &fakeRestoreEngine{volumeProgress: map[string]int64{}}
	for i, size := range sizes {
		name := fmt.Sprintf("disk-%d", i)
		vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups, harvesterv1.VolumeBackup{
			Name: ptr.To("backup-" + name), VolumeName: name, VolumeSize: size,
		})
		re.volumeProgress[name] = progress[i]
	}
	vmro := restorecommon.NewVMRestoreOperatorBuilder().WithClient(client).
		WithVMBackupOperator(backupcommon.NewVMBackupOperatorBuilder().Build()).Build()
	h := &RestoreHandler{
		vmro: vmro, engines: map[harvesterv1.BackupType]engine.RestoreEngine{kind: re},
	}
	return h, vmb, client
}

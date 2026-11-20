package virtualmachinebackup

import (
	"testing"

	"github.com/stretchr/testify/require"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

func TestPendingTargetAdmission(t *testing.T) {
	for _, scenario := range []string{"unmasked", "pending", "unverified", "replaced secret"} {
		t.Run(scenario, func(t *testing.T) {
			v := gateValidator(`{"type":"s3","bucketName":"backups","thirdPartyBackupEnabled":true}`)
			setting, target, err := v.targets.Get()
			require.NoError(t, err)
			switch scenario {
			case "unmasked":
				setting.Value = `{"type":"s3","bucketName":"backups","thirdPartyBackupEnabled":true,"accessKeyId":"rotated","secretAccessKey":"secret","thirdPartyRepositoryPassword":"password"}`
			case "pending":
				harvesterv1.SettingConfigured.False(setting)
			case "unverified":
				delete(setting.Annotations, backuputil.ReadyAnnotation)
			case "replaced secret":
				secret, err := v.targets.ReadyRepoSecret(setting, target)
				require.NoError(t, err)
				secret.UID = "replacement"
				_, err = v.targets.Secrets.Update(secret)
				require.NoError(t, err)
			}
			_, err = v.targets.Settings.Update(setting)
			require.NoError(t, err)
			for _, kind := range []harvesterv1.BackupType{harvesterv1.Backup, harvesterv1.Restic, harvesterv1.Kopia} {
				require.Error(t, v.Create(nil, newVMBackup("default", "test-vm", kind)), "%s", kind)
			}
			require.NoError(t, v.Create(nil, newVMBackup("default", "test-vm", harvesterv1.Snapshot)))
		})
	}
}

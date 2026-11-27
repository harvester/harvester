package virtualmachinerestore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestRestoreReadiness(t *testing.T) {
	for _, kind := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia} {
		t.Run(string(kind), func(t *testing.T) {
			target := &settings.BackupTarget{Type: settings.S3BackupType, BucketName: "backups", ThirdPartyBackupEnabled: true}
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
				UID: "original", ResourceVersion: "1",
			}, Data: map[string][]byte{
				util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret"),
				backuputil.ResticPasswordKey: []byte("password"), backuputil.KopiaPasswordKey: []byte("password"),
				backuputil.RepositoryIDKey: []byte(backuputil.RepositoryID(target)),
			}}
			record, err := backuputil.RepoReadyRecord(target, secret)
			require.NoError(t, err)
			value, err := json.Marshal(target)
			require.NoError(t, err)
			setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
				Name: settings.BackupTargetSettingName, Annotations: map[string]string{backuputil.ReadyAnnotation: record},
			}, Value: string(value)}
			harvesterv1.SettingConfigured.True(setting)
			client := fake.NewSimpleClientset(setting, secret)
			v := &restoreValidator{vmbr: backupcommon.NewVMBackupReader(), targets: backuputil.TargetClient{
				Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
				Secrets:  fakeclients.SecretClient(client.CoreV1().Secrets),
			}}
			vmb := &harvesterv1.VirtualMachineBackup{
				Spec:   harvesterv1.VirtualMachineBackupSpec{Type: kind},
				Status: harvesterv1.VirtualMachineBackupStatus{BackupTarget: backuputil.BackupTargetStatus(target)},
			}
			require.NoError(t, v.checkDataMoverBackup(vmb))
			secret.ResourceVersion = "2"
			_, err = v.targets.Secrets.Update(secret)
			require.NoError(t, err)
			require.Error(t, v.checkDataMoverBackup(vmb), "a replaced credential revision needs validation")
			target.AccessKeyID, target.SecretAccessKey = "rotated", "rotated"
			value, err = json.Marshal(target)
			require.NoError(t, err)
			setting.Value = string(value)
			_, err = v.targets.Settings.Update(setting)
			require.NoError(t, err)
			require.ErrorContains(t, v.checkDataMoverBackup(vmb), "pending")
		})
	}
}

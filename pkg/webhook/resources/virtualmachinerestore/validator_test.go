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
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckDataMoverBackup(t *testing.T) {
	target := &settings.BackupTarget{
		Type:                    settings.S3BackupType,
		Endpoint:                "https://s3.example.com",
		BucketName:              "backups",
		BucketRegion:            "region-a",
		ThirdPartyBackupEnabled: true,
	}
	targetValue, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName},
		Value:      string(targetValue),
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "harvester-backup-target-secret", Namespace: "longhorn-system", UID: "source-uid", ResourceVersion: "1",
	}, Data: map[string][]byte{
		"AWS_ACCESS_KEY_ID": []byte("access"), "AWS_SECRET_ACCESS_KEY": []byte("secret"),
		backuputil.ResticPasswordKey: []byte("password"), backuputil.RepositoryIDKey: []byte(backuputil.RepositoryID(target)),
	}}
	record, err := backuputil.RepoReadyRecord(target, secret)
	require.NoError(t, err)
	setting.Annotations = map[string]string{backuputil.ReadyAnnotation: record}
	harvesterv1.SettingConfigured.True(setting)
	clientset := fake.NewSimpleClientset(setting, secret)
	validator := &restoreValidator{
		setting: fakeclients.HarvesterSettingCache(
			clientset.HarvesterhciV1beta1().Settings,
		),
		vmbr:    backupcommon.NewVMBackupReader(),
		targets: backuputil.TargetClient{Settings: fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings), Secrets: fakeclients.SecretClient(clientset.CoreV1().Secrets)},
	}
	vmBackup := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
		Spec:       harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
		Status: harvesterv1.VirtualMachineBackupStatus{
			BackupTarget: backuputil.BackupTargetStatus(target),
		},
	}

	require.NoError(t, validator.checkDataMoverBackup(vmBackup))

	repositoryID := vmBackup.Status.BackupTarget.RepositoryID
	vmBackup.Status.BackupTarget.RepositoryID = ""
	require.ErrorContains(t,
		validator.checkDataMoverBackup(vmBackup),
		"does not record a repository ID",
	)
	vmBackup.Status.BackupTarget.RepositoryID = repositoryID

	otherTarget := *target
	otherTarget.BucketName = "other"
	otherValue, err := json.Marshal(&otherTarget)
	require.NoError(t, err)
	setting.Value = string(otherValue)
	clientset = fake.NewSimpleClientset(setting)
	validator.targets.Settings = fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings)
	validator.setting = fakeclients.HarvesterSettingCache(
		clientset.HarvesterhciV1beta1().Settings,
	)
	require.ErrorContains(t,
		validator.checkDataMoverBackup(vmBackup),
		"does not match",
	)
}

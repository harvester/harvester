package backup_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func readyFixture(t *testing.T) (*harvesterv1.Setting, *settings.BackupTarget, *corev1.Secret) {
	t.Helper()
	target := &settings.BackupTarget{Type: settings.S3BackupType, BucketName: "bucket", ThirdPartyBackupEnabled: true}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
		UID: "original", ResourceVersion: "1",
	}, Data: map[string][]byte{
		util.AWSAccessKey: []byte("private-access"), util.AWSSecretKey: []byte("private-secret"),
		backuputil.ResticPasswordKey: []byte("private-password"),
		backuputil.RepositoryIDKey:   []byte(backuputil.RepositoryID(target)),
	}}
	record, err := backuputil.RepoReadyRecord(target, secret)
	require.NoError(t, err)
	require.NotContains(t, record, "private-")
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
		Name: settings.BackupTargetSettingName, Annotations: map[string]string{backuputil.ReadyAnnotation: record},
	}, Value: string(value)}
	harvesterv1.SettingConfigured.True(setting)
	return setting, target, secret
}

func TestReadinessInvalidation(t *testing.T) {
	for _, action := range []string{"pending", "deleting", "resubmitted", "rotation", "replacement", "modified secret", "target change", "metadata only"} {
		t.Run(action, func(t *testing.T) {
			setting, target, secret := readyFixture(t)
			switch action {
			case "deleting":
				now := metav1.Now()
				setting.DeletionTimestamp = &now
			case "pending":
				harvesterv1.SettingConfigured.False(setting)
			case "resubmitted":
				target.RepoPassword = "private-password"
			case "rotation":
				target.AccessKeyID, target.SecretAccessKey = "new", "new"
			case "replacement":
				secret.UID = "replacement"
			case "modified secret":
				secret.ResourceVersion = "2"
			case "target change":
				target.BucketRegion = "new"
			case "metadata only":
				setting.ResourceVersion = "20"
				setting.Annotations["unrelated"] = "value"
			}
			err := backuputil.CheckRepoReady(setting, target, secret)
			if action == "metadata only" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestLiveTargetCredentials(t *testing.T) {
	setting, target, secret := readyFixture(t)
	client := fake.NewSimpleClientset(setting, secret)
	reader := backuputil.TargetClient{
		Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
		Secrets:  fakeclients.SecretClient(client.CoreV1().Secrets),
	}
	status := backuputil.BackupTargetStatus(target)
	data, err := reader.ThirdPartyCredentials(status, backuputil.ResticPasswordKey)
	require.NoError(t, err)
	require.Len(t, data, 3)
	// Updating only the live Setting invalidates readiness, without a cache event.
	target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "new", "new", "private-password"
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting.Value = string(value)
	_, err = client.HarvesterhciV1beta1().Settings().Update(context.Background(), setting, metav1.UpdateOptions{})
	require.NoError(t, err)
	_, err = reader.ThirdPartyCredentials(status, backuputil.ResticPasswordKey)
	require.ErrorContains(t, err, "pending")
}

func TestReceiptDoesNotAuthorizeJobs(t *testing.T) {
	setting, target, secret := readyFixture(t)
	harvesterv1.SettingConfigured.False(setting)
	require.NoError(t, backuputil.CheckRepoReceipt(setting, target, secret))
	require.ErrorContains(t, backuputil.CheckRepoReady(setting, target, secret), "pending")
	secret.ResourceVersion = "new-version"
	require.Error(t, backuputil.CheckRepoReceipt(setting, target, secret))
}

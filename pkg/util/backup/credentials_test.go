package backup_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCredentialDataRejectsInvalidSource(t *testing.T) {
	for _, passwordKey := range []string{"RESTIC_PASSWORD", "KOPIA_PASSWORD"} {
		for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, passwordKey, backuputil.RepositoryIDKey} {
			t.Run(passwordKey+"/"+key, func(t *testing.T) {
				client, target := credentialFixture(t)
				secrets := fakeclients.SecretClient(client.CoreV1().Secrets)
				source, err := secrets.Get(
					util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{},
				)
				require.NoError(t, err)
				delete(source.Data, key)
				_, err = secrets.Update(source)
				require.NoError(t, err)
				_, err = credentialReader(client).ThirdPartyCredentials(target, passwordKey)
				require.Error(t, err)
			})
		}
	}
	t.Run("wrong repository", func(t *testing.T) {
		client, target := credentialFixture(t)
		secrets := fakeclients.SecretClient(client.CoreV1().Secrets)
		source, err := secrets.Get(
			util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{},
		)
		require.NoError(t, err)
		source.Data[backuputil.RepositoryIDKey] = []byte("another-repository")
		_, err = secrets.Update(source)
		require.NoError(t, err)
		_, err = credentialReader(client).ThirdPartyCredentials(target, "RESTIC_PASSWORD")
		require.ErrorContains(t, err, "repository ID does not match")
	})
	t.Run("missing secret", func(t *testing.T) {
		client, target := credentialFixture(t)
		require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
			context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
		))
		_, err := credentialReader(client).ThirdPartyCredentials(target, "RESTIC_PASSWORD")
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestCredentialDataChecksTargetBeforeReadingSecret(t *testing.T) {
	client, target := credentialFixture(t)
	setting, err := client.HarvesterhciV1beta1().Settings().Get(context.Background(), settings.BackupTargetSettingName, metav1.GetOptions{})
	require.NoError(t, err)
	setting.Value = `{"type":"s3","bucketName":"other"}`
	_, err = client.HarvesterhciV1beta1().Settings().Update(context.Background(), setting, metav1.UpdateOptions{})
	require.NoError(t, err)
	client.ClearActions()
	_, err = credentialReader(client).ThirdPartyCredentials(target, "RESTIC_PASSWORD")
	require.ErrorContains(t, err, "does not match the configured backup target")
	require.Len(t, client.Actions(), 1)
	require.Equal(t, "settings", client.Actions()[0].GetResource().Resource)
}

func credentialFixture(t *testing.T) (*fake.Clientset, *harvesterv1.BackupTarget) {
	t.Helper()
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	value := `{"type":"s3","endpoint":"https://s3.example.com","bucketName":"backups","thirdPartyBackupEnabled":true}`
	require.NoError(t, settings.BackupTargetSet.Set(value))
	target, err := settings.DecodeBackupTarget(value)
	require.NoError(t, err)
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName, UID: "source", ResourceVersion: "1",
		},
		Data: map[string][]byte{
			util.AWSAccessKey:          []byte("access"),
			util.AWSSecretKey:          []byte("secret"),
			"RESTIC_PASSWORD":          []byte("stored-RESTIC_PASSWORD"),
			"KOPIA_PASSWORD":           []byte("stored-KOPIA_PASSWORD"),
			backuputil.RepositoryIDKey: []byte(backuputil.RepositoryID(target)),
			util.AWSEndpoints:          []byte("https://s3.example.com"),
		},
	}
	record, err := backuputil.RepoReadyRecord(target, source)
	require.NoError(t, err)
	encoded, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
		Name: settings.BackupTargetSettingName, Annotations: map[string]string{backuputil.ReadyAnnotation: record},
	}, Value: string(encoded)}
	harvesterv1.SettingConfigured.True(setting)
	return fake.NewSimpleClientset(source, setting), backuputil.BackupTargetStatus(target)
}

func TestCredentialDataPreservesLiteralValues(t *testing.T) {
	client, target := credentialFixture(t)
	secrets := fakeclients.SecretClient(client.CoreV1().Secrets)
	source, err := secrets.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	values := map[string][]byte{
		util.AWSAccessKey: []byte(`access$(PATH)$$`), util.AWSSecretKey: []byte(`secret$(AWS_ACCESS_KEY_ID)$`),
		backuputil.ResticPasswordKey: []byte(`password$$$(AWS_SECRET_ACCESS_KEY)`),
	}
	for k, v := range values {
		source.Data[k] = v
	}
	_, err = secrets.Update(source)
	require.NoError(t, err)
	data, err := credentialReader(client).ThirdPartyCredentials(target, backuputil.ResticPasswordKey)
	require.NoError(t, err)
	require.Equal(t, values, data)
	for key := range values {
		old := source.Data[key]
		source.Data[key] = []byte("private\x00value")
		_, err = secrets.Update(source)
		require.NoError(t, err)
		_, err = credentialReader(client).ThirdPartyCredentials(target, backuputil.ResticPasswordKey)
		require.ErrorContains(t, err, "invalid "+key)
		require.NotContains(t, err.Error(), "private")
		source.Data[key] = old
	}
	_, err = credentialReader(client).ThirdPartyCredentials(target, util.AWSSecretKey)
	require.ErrorContains(t, err, "unsupported")
}

func credentialReader(client *fake.Clientset) backuputil.TargetClient {
	return backuputil.TargetClient{
		Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
		Secrets:  fakeclients.SecretClient(client.CoreV1().Secrets),
	}
}

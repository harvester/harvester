package backup_test

import (
	"context"
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
				_, err = backuputil.CredentialData(secrets, target, passwordKey)
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
		_, err = backuputil.CredentialData(secrets, target, "RESTIC_PASSWORD")
		require.ErrorContains(t, err, "repository ID does not match")
	})
	t.Run("missing secret", func(t *testing.T) {
		client, target := credentialFixture(t)
		require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
			context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
		))
		_, err := backuputil.CredentialData(
			fakeclients.SecretClient(client.CoreV1().Secrets), target, "RESTIC_PASSWORD",
		)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestCredentialDataChecksTargetBeforeReadingSecret(t *testing.T) {
	client, target := credentialFixture(t)
	require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","bucketName":"other"}`))
	client.ClearActions()
	_, err := backuputil.CredentialData(
		fakeclients.SecretClient(client.CoreV1().Secrets), target, "RESTIC_PASSWORD",
	)
	require.ErrorContains(t, err, "does not match the configured backup target")
	require.Empty(t, client.Actions())
}

func credentialFixture(t *testing.T) (*fake.Clientset, *harvesterv1.BackupTarget) {
	t.Helper()
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	value := `{"type":"s3","endpoint":"https://s3.example.com","bucketName":"backups"}`
	require.NoError(t, settings.BackupTargetSet.Set(value))
	target, err := settings.DecodeBackupTarget(value)
	require.NoError(t, err)
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
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
	return fake.NewSimpleClientset(source), backuputil.BackupTargetStatus(target)
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
	data, err := backuputil.CredentialData(secrets, target, backuputil.ResticPasswordKey)
	require.NoError(t, err)
	require.Equal(t, values, data)
	for key := range values {
		old := source.Data[key]
		source.Data[key] = []byte("private\x00value")
		_, err = secrets.Update(source)
		require.NoError(t, err)
		_, err = backuputil.CredentialData(secrets, target, backuputil.ResticPasswordKey)
		require.ErrorContains(t, err, "invalid "+key)
		require.NotContains(t, err.Error(), "private")
		source.Data[key] = old
	}
	_, err = backuputil.CredentialData(secrets, target, util.AWSSecretKey)
	require.ErrorContains(t, err, "unsupported")
}

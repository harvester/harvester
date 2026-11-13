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

func TestCheckCredentialsRejectsInvalidSource(t *testing.T) {
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
				err = backuputil.CheckCredentials(secrets, target, passwordKey)
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
		err = backuputil.CheckCredentials(secrets, target, "RESTIC_PASSWORD")
		require.ErrorContains(t, err, "repository ID does not match")
	})
	t.Run("missing secret", func(t *testing.T) {
		client, target := credentialFixture(t)
		require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
			context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
		))
		err := backuputil.CheckCredentials(
			fakeclients.SecretClient(client.CoreV1().Secrets), target, "RESTIC_PASSWORD",
		)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestCheckCredentialsChecksTargetBeforeReadingSecret(t *testing.T) {
	client, target := credentialFixture(t)
	require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","bucketName":"other"}`))
	client.ClearActions()
	err := backuputil.CheckCredentials(
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

func TestCredentialEnvPreservesLiteralValues(t *testing.T) {
	data := map[string][]byte{
		backuputil.RepositoryIDKey: []byte("repo"),
		util.AWSAccessKey:          []byte(`access$(PATH)$$`),
		util.AWSSecretKey:          []byte(`secret$(AWS_ACCESS_KEY_ID)$$(HOME)$`),
		"RESTIC_PASSWORD":          []byte(`password$$$(AWS_SECRET_ACCESS_KEY)`),
	}
	env, err := backuputil.CredentialEnv(data, "repo", "RESTIC_PASSWORD")
	require.NoError(t, err)
	require.Equal(t, []string{
		`AWS_ACCESS_KEY_ID=access$(PATH)$$`,
		`AWS_SECRET_ACCESS_KEY=secret$(AWS_ACCESS_KEY_ID)$$(HOME)$`,
		`RESTIC_PASSWORD=password$$$(AWS_SECRET_ACCESS_KEY)`,
	}, env)
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, "RESTIC_PASSWORD"} {
		t.Run(key, func(t *testing.T) {
			old := data[key]
			data[key] = []byte("private\x00value")
			env, err := backuputil.CredentialEnv(data, "repo", "RESTIC_PASSWORD")
			require.ErrorContains(t, err, "invalid "+key)
			require.NotContains(t, err.Error(), "private")
			require.Nil(t, env)
			data[key] = old
		})
	}
	_, err = backuputil.CredentialEnv(data, "repo", util.AWSSecretKey)
	require.ErrorContains(t, err, "unsupported")
}

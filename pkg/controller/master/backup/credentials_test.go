package backup

import (
	"context"
	"encoding/json"
	"testing"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

func TestCredentialsInitializeOnUpgrade(t *testing.T) {
	for _, interval := range []string{"", `,"refreshIntervalInSeconds":0`, `,"refreshIntervalInSeconds":null`} {
		t.Run(interval, func(t *testing.T) {
			h, setting, target, client := credentialHandler(t)
			setting.Value = `{"type":"s3","endpoint":"http://minio.example.com:9000",` +
				`"bucketName":"backups","bucketRegion":"test-region"` + interval + `}`
			original := readCredentials(t, h)
			updated, err := h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.True(t, harvesterv1.SettingConfigured.IsTrue(updated))
			require.Equal(t, setting, updated, "initialization must not require a Setting change")
			initialized := readCredentials(t, h)
			for key, value := range original.Data {
				require.Equal(t, value, initialized.Data[key], key)
			}
			assertInitialPasswords(t, initialized.Data, target)

			client.ClearActions()
			_, err = h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.Equal(t, initialized.Data, readCredentials(t, h).Data)
			for _, action := range client.Actions() {
				require.False(t, action.Matches("update", "secrets"), "repeat reconciliation must be a no-op")
			}
		})
	}
}

func TestCredentialsSurviveRotation(t *testing.T) {
	h, setting, target, _ := credentialHandler(t)
	require.NoError(t, h.syncRepositoryPasswords(target))
	initial := readCredentials(t, h)

	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey} {
		if key == util.AWSAccessKey {
			target.AccessKeyID = " rotated-access "
			target.SecretAccessKey = " secret "
		} else {
			target.AccessKeyID = " rotated-access "
			target.SecretAccessKey = " rotated-secret "
		}
		value, err := json.Marshal(target)
		require.NoError(t, err)
		setting.Value = string(value)
		setting, err = h.OnBackupTargetChange(setting.Name, setting)
		require.NoError(t, err)
		require.True(t, harvesterv1.SettingConfigured.IsTrue(setting))
		stripped, err := settings.DecodeBackupTarget(setting.Value)
		require.NoError(t, err)
		require.Empty(t, stripped.AccessKeyID)
		require.Empty(t, stripped.SecretAccessKey)
		_, err = h.OnBackupTargetChange(setting.Name, setting)
		require.NoError(t, err)
		current := readCredentials(t, h)
		require.Equal(t, []byte("rotated-access"), current.Data[util.AWSAccessKey])
		if key == util.AWSSecretKey {
			require.Equal(t, []byte("rotated-secret"), current.Data[util.AWSSecretKey])
		}
		for _, field := range []string{backuputil.RepositoryIDKey, resticutil.PasswordKey, kopiautil.PasswordKey} {
			require.Equal(t, initial.Data[field], current.Data[field], field)
		}
	}
}

func TestCredentialsChangeWithRepository(t *testing.T) {
	h, _, target, _ := credentialHandler(t)
	require.NoError(t, h.syncRepositoryPasswords(target))
	initial := readCredentials(t, h)
	target.BucketName = "another-bucket"
	target.AccessKeyID, target.SecretAccessKey = " access ", " secret "
	require.NoError(t, h.syncBackupSecret(target))
	current := readCredentials(t, h)
	assertInitialPasswords(t, current.Data, target)
	require.Equal(t, []byte("access"), current.Data[util.AWSAccessKey])
	require.Equal(t, []byte("secret"), current.Data[util.AWSSecretKey])
	for _, key := range []string{backuputil.RepositoryIDKey, resticutil.PasswordKey, kopiautil.PasswordKey} {
		require.NotEqual(t, initial.Data[key], current.Data[key], key)
	}
}

func TestCredentialsRejectIncompleteState(t *testing.T) {
	for _, state := range []string{"missing-restic", "missing-kopia", "missing-both", "missing-id", "partial-legacy",
		"wrong-id", "missing-access", "missing-secret"} {
		t.Run(state, func(t *testing.T) {
			h, _, target, client := credentialHandler(t)
			require.NoError(t, h.syncRepositoryPasswords(target))
			secret := readCredentials(t, h)
			switch state {
			case "missing-restic":
				delete(secret.Data, resticutil.PasswordKey)
			case "missing-kopia":
				delete(secret.Data, kopiautil.PasswordKey)
			case "missing-both":
				delete(secret.Data, resticutil.PasswordKey)
				delete(secret.Data, kopiautil.PasswordKey)
			case "missing-id":
				delete(secret.Data, backuputil.RepositoryIDKey)
			case "partial-legacy":
				delete(secret.Data, backuputil.RepositoryIDKey)
				delete(secret.Data, resticutil.PasswordKey)
			case "wrong-id":
				secret.Data[backuputil.RepositoryIDKey] = []byte("other")
			case "missing-access":
				delete(secret.Data, util.AWSAccessKey)
			case "missing-secret":
				delete(secret.Data, util.AWSSecretKey)
			}
			_, err := h.secrets.Update(secret)
			require.NoError(t, err)
			client.ClearActions()
			require.Error(t, h.syncRepositoryPasswords(target))
			if state != "wrong-id" && state != "missing-access" && state != "missing-secret" {
				target.AccessKeyID, target.SecretAccessKey = "new-access", "new-secret"
				require.Error(t, h.syncBackupSecret(target))
			}
			require.Equal(t, secret.Data, readCredentials(t, h).Data)
			for _, action := range client.Actions() {
				require.False(t, action.Matches("update", "secrets"))
			}
		})
	}
}

func TestCredentialsInitializeForNewTarget(t *testing.T) {
	h, _, target, _ := credentialHandler(t)
	require.NoError(t, h.secrets.Delete(
		util.LonghornSystemNamespaceName, util.BackupTargetSecretName, &metav1.DeleteOptions{},
	))
	target.AccessKeyID, target.SecretAccessKey = " access ", " secret "
	require.NoError(t, h.syncBackupSecret(target))
	secret := readCredentials(t, h)
	assertInitialPasswords(t, secret.Data, target)
	require.Equal(t, []byte("access"), secret.Data[util.AWSAccessKey])
	require.Equal(t, []byte("secret"), secret.Data[util.AWSSecretKey])
}

func TestCredentialsClearWithTarget(t *testing.T) {
	for _, value := range []string{"", `{"type":"nfs","endpoint":"nfs.test:/backups"}`} {
		t.Run(value, func(t *testing.T) {
			h, setting, target, _ := credentialHandler(t)
			require.NoError(t, h.syncRepositoryPasswords(target))
			setting.Value = value
			_, err := h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.Empty(t, readCredentials(t, h).Data)
		})
	}
}

func assertInitialPasswords(t *testing.T, data map[string][]byte, target *settings.BackupTarget) {
	t.Helper()
	id := backuputil.RepositoryID(target)
	require.Equal(t, []byte(id), data[backuputil.RepositoryIDKey])
	for backupType, key := range map[harvesterv1.BackupType]string{
		harvesterv1.Restic: resticutil.PasswordKey, harvesterv1.Kopia: kopiautil.PasswordKey,
	} {
		expected, err := backuputil.RepositoryPassword(id, backupType, data)
		require.NoError(t, err)
		require.Equal(t, []byte(expected), data[key])
	}
}

func readCredentials(t *testing.T, h *TargetHandler) *corev1.Secret {
	t.Helper()
	secret, err := h.secrets.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	return secret
}

func credentialHandler(t *testing.T) (*TargetHandler, *harvesterv1.Setting, *settings.BackupTarget, *fake.Clientset) {
	t.Helper()
	target := kopiaMaintenanceTarget(false)
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName}, Value: string(value),
	}
	harvesterv1.SettingConfigured.True(setting)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName},
		Data: map[string][]byte{
			util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret"),
			util.AWSCERT: []byte("certificate"), util.HTTPProxyEnv: []byte("http://proxy.test"),
			"custom": []byte("preserved"),
		},
	}
	lhTarget := &longhornv1.BackupTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: longhorntypes.DefaultBackupTargetName, Namespace: util.LonghornSystemNamespaceName,
		},
		Spec: longhornv1.BackupTargetSpec{CredentialSecret: util.BackupTargetSecretName},
	}
	client := fake.NewSimpleClientset(secret, setting, lhTarget)
	h := &TargetHandler{
		ctx: context.Background(), clientset: kubefake.NewSimpleClientset(),
		secrets:             fakeclients.SecretClient(client.CoreV1().Secrets),
		secretCache:         fakeclients.SecretCache(client.CoreV1().Secrets),
		settings:            fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
		lhBackupTargetCache: credentialTargetCache{client: client},
		lhBackupTargets:     credentialTargetClient{client: client},
	}
	return h, setting, target, client
}

type credentialTargetCache struct {
	ctllonghornv1.BackupTargetCache
	client *fake.Clientset
}

func (c credentialTargetCache) Get(namespace, name string) (*longhornv1.BackupTarget, error) {
	return c.client.LonghornV1beta2().BackupTargets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

type credentialTargetClient struct {
	ctllonghornv1.BackupTargetClient
	client *fake.Clientset
}

func (c credentialTargetClient) Update(target *longhornv1.BackupTarget) (*longhornv1.BackupTarget, error) {
	return c.client.LonghornV1beta2().BackupTargets(target.Namespace).Update(
		context.Background(), target, metav1.UpdateOptions{},
	)
}

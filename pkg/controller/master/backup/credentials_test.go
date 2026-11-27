package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

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

func TestLegacyCredentialsSurviveUpgrade(t *testing.T) {
	for _, interval := range []string{"", `,"refreshIntervalInSeconds":0`, `,"refreshIntervalInSeconds":null`} {
		t.Run(interval, func(t *testing.T) {
			h, setting, _, client := credentialHandler(t)
			setting.Value = `{"type":"s3","endpoint":"http://minio.example.com:9000",` +
				`"bucketName":"backups","bucketRegion":"test-region"` + interval + `}`
			original := readCredentials(t, h)
			for range 2 {
				client.ClearActions()
				updated, err := h.OnBackupTargetChange(setting.Name, setting)
				require.NoError(t, err)
				require.Equal(t, setting, updated)
				for _, action := range client.Actions() {
					require.Equal(t, "get", action.GetVerb(),
						"masked reconciliation must not change existing Longhorn credentials")
				}
				require.Equal(t, original, readCredentials(t, h))
			}
		})
	}
}

func TestTargetPasswordSync(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		t.Run(fmt.Sprintf("newSecret=%t", fresh), func(t *testing.T) {
			h, setting, target, client := credentialHandler(t)
			if fresh {
				require.NoError(t, h.secrets.Delete(
					util.LonghornSystemNamespaceName, util.BackupTargetSecretName, &metav1.DeleteOptions{},
				))
			}
			target.AccessKeyID, target.SecretAccessKey = " access ", " secret "
			target.ThirdPartyBackupEnabled = true
			target.RepoPassword = "  repo$$$(AWS_SECRET_ACCESS_KEY)  "
			setTargetValue(t, h, setting, target)
			updated, err := h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.NotContains(t, updated.Value, "thirdPartyRepositoryPassword")
			require.Contains(t, setting.Value, target.RepoPassword, "cached input must not be mutated")
			saved := readCredentials(t, h)
			assertRepoPassword(t, saved, target.RepoPassword)
			require.Equal(t, backuputil.RepositoryID(target), string(saved.Data[backuputil.RepositoryIDKey]))
			require.Equal(t, []byte("access"), saved.Data[util.AWSAccessKey])
			require.Equal(t, []byte("secret"), saved.Data[util.AWSSecretKey])
			if !fresh {
				require.Equal(t, []byte("preserved"), saved.Data["custom"])
			}
			client.ClearActions()
			_, err = h.OnBackupTargetChange(updated.Name, updated)
			require.NoError(t, err)
			require.Equal(t, saved.Data, readCredentials(t, h).Data)
			for _, action := range client.Actions() {
				require.False(t, action.Matches("update", "secrets"))
			}
		})
	}
}

func TestCredentialsSurviveRotation(t *testing.T) {
	h, setting, target, _ := credentialHandler(t)
	initRepo(t, h, target)
	initial := readCredentials(t, h)
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey} {
		target.AccessKeyID, target.SecretAccessKey = " rotated-access ", " secret "
		if key == util.AWSSecretKey {
			target.SecretAccessKey = " rotated-secret "
		}
		setTargetValue(t, h, setting, target)
		updated, err := h.OnBackupTargetChange(setting.Name, setting)
		require.NoError(t, err)
		stripped, err := settings.DecodeBackupTarget(updated.Value)
		require.NoError(t, err)
		require.Empty(t, stripped.AccessKeyID)
		require.Empty(t, stripped.SecretAccessKey)
		require.Empty(t, stripped.RepoPassword)
		require.True(t, stripped.ThirdPartyBackupEnabled)
		_, err = h.OnBackupTargetChange(updated.Name, updated)
		require.NoError(t, err)
		current := readCredentials(t, h)
		require.Equal(t, []byte("rotated-access"), current.Data[util.AWSAccessKey])
		if key == util.AWSSecretKey {
			require.Equal(t, []byte("rotated-secret"), current.Data[util.AWSSecretKey])
		}
		for _, field := range []string{backuputil.RepositoryIDKey, resticutil.PasswordKey, kopiautil.PasswordKey, "custom"} {
			require.Equal(t, initial.Data[field], current.Data[field], field)
		}
	}
}

func TestRepositoryChange(t *testing.T) {
	h, _, target, _ := credentialHandler(t)
	initRepo(t, h, target)
	initial := readCredentials(t, h)
	target.BucketName = "another-bucket"
	target.AccessKeyID, target.SecretAccessKey = " access ", " secret "
	target.RepoPassword = "another-password"
	require.NoError(t, syncTestTarget(t, h, target))
	current := readCredentials(t, h)
	require.Equal(t, backuputil.RepositoryID(target), string(current.Data[backuputil.RepositoryIDKey]))
	require.NotEqual(t, initial.Data[backuputil.RepositoryIDKey], current.Data[backuputil.RepositoryIDKey])
	assertRepoPassword(t, current, target.RepoPassword)
	require.Equal(t, initial.Data["custom"], current.Data["custom"])
}

func TestDisabledClearsPasswords(t *testing.T) {
	h, setting, target, client := credentialHandler(t)
	initRepo(t, h, target)
	target.ThirdPartyBackupEnabled, target.RepoPassword = false, ""
	target.AccessKeyID, target.SecretAccessKey = "access", "secret"
	setTargetValue(t, h, setting, target)
	updated, err := h.OnBackupTargetChange(setting.Name, setting)
	require.NoError(t, err)
	secret := readCredentials(t, h)
	assertRepoPassword(t, secret, "")
	require.Equal(t, []byte("access"), secret.Data[util.AWSAccessKey])
	require.Equal(t, backuputil.RepositoryID(target), string(secret.Data[backuputil.RepositoryIDKey]))

	client.ClearActions()
	_, err = h.OnBackupTargetChange(updated.Name, updated)
	require.NoError(t, err)
	require.Equal(t, secret.Data, readCredentials(t, h).Data)
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb(), "masked reconciliation must only read stored credentials")
	}

	target.ThirdPartyBackupEnabled, target.RepoPassword = true, "replacement"
	target.AccessKeyID, target.SecretAccessKey = "access", "secret"
	require.NoError(t, syncTestTarget(t, h, target))
	assertRepoPassword(t, readCredentials(t, h), "replacement")
}

func TestMaskedTargetPreservesPasswords(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			h, setting, target, client := credentialHandler(t)
			initRepo(t, h, target)
			// A masked Setting must not trigger password writes, even if the Secret
			// contains passwords while third-party backups are disabled.
			target.ThirdPartyBackupEnabled = enabled
			target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "", "", ""
			setTargetValue(t, h, setting, target)
			secret := readCredentials(t, h)
			client.ClearActions()
			_, err := h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.Equal(t, secret.Data, readCredentials(t, h).Data)
			for _, action := range client.Actions() {
				if action.GetResource().Resource == "secrets" {
					require.Equal(t, "get", action.GetVerb(), "masked reconciliation must not write credentials")
				}
			}
		})
	}
}

func TestMaskedTargetRequiresAWS(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tt := range []struct {
			name, key string
			missing   bool
		}{
			{"missing Secret", "", true},
			{"read failure", "", false},
			{"missing access", util.AWSAccessKey, true},
			{"empty access", util.AWSAccessKey, false},
			{"missing secret key", util.AWSSecretKey, true},
			{"empty secret key", util.AWSSecretKey, false},
		} {
			t.Run(fmt.Sprintf("enabled=%t/%s", enabled, tt.name), func(t *testing.T) {
				h, _, target, client := credentialHandler(t)
				if enabled {
					initRepo(t, h, target)
				}
				target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "", "", ""
				secret := readCredentials(t, h)
				switch tt.name {
				case "missing Secret":
					require.NoError(t, h.secrets.Delete(secret.Namespace, secret.Name, &metav1.DeleteOptions{}))
				case "read failure":
					client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
						return true, nil, fmt.Errorf("injected read failure")
					})
				default:
					secret.Data[tt.key] = []byte{}
					if tt.missing {
						delete(secret.Data, tt.key)
					}
					_, err := h.secrets.Update(secret)
					require.NoError(t, err)
				}
				client.ClearActions()
				_, err := h.checkSavedSecret(target)
				require.Error(t, err)
				if tt.name == "read failure" {
					require.ErrorContains(t, err, "injected read failure")
				}
				for _, action := range client.Actions() {
					require.Equal(t, "get", action.GetVerb(), "credential validation must not change resources")
				}
			})
		}
	}
}

func TestTargetResetClearsPasswords(t *testing.T) {
	for _, value := range []string{"", settings.InitBackupTargetToString(), `{"type":"nfs","endpoint":"nfs.test:/backups"}`} {
		t.Run(value, func(t *testing.T) {
			h, setting, target, _ := credentialHandler(t)
			initRepo(t, h, target)
			setting.Value = value
			for range 2 {
				_, err := h.OnBackupTargetChange(setting.Name, setting)
				require.NoError(t, err)
				require.Equal(t, map[string][]byte{"custom": []byte("preserved")}, readCredentials(t, h).Data)
				lhTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
				require.NoError(t, err)
				require.Empty(t, lhTarget.Spec.CredentialSecret)
			}
		})
	}
}

func TestInvalidCredentialsStayUnchanged(t *testing.T) {
	for _, state := range []string{
		"replacement", "missing restic", "missing kopia", "inconsistent passwords", "missing access",
		"missing secret", "wrong repository", "new repository without password", "disabled with password",
		"missing repository ID", "missing passwords",
		"input without AWS", "input without access", "input without secret",
	} {
		t.Run(state, func(t *testing.T) {
			h, _, target, client := credentialHandler(t)
			initRepo(t, h, target)
			// Reconcile masked credentials unless explicitly submitting input below.
			target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "", "", ""
			secret := readCredentials(t, h)
			switch state {
			case "replacement":
				target.RepoPassword = "different"
				target.AccessKeyID, target.SecretAccessKey = "access", "secret"
			case "missing restic":
				delete(secret.Data, resticutil.PasswordKey)
			case "missing kopia":
				delete(secret.Data, kopiautil.PasswordKey)
			case "inconsistent passwords":
				secret.Data[kopiautil.PasswordKey] = []byte("different")
			case "missing access":
				delete(secret.Data, util.AWSAccessKey)
			case "missing secret":
				delete(secret.Data, util.AWSSecretKey)
			case "wrong repository":
				secret.Data[backuputil.RepositoryIDKey] = []byte("other")
			case "missing repository ID":
				delete(secret.Data, backuputil.RepositoryIDKey)
			case "missing passwords":
				delete(secret.Data, resticutil.PasswordKey)
				delete(secret.Data, kopiautil.PasswordKey)
			case "new repository without password":
				target.BucketName, target.RepoPassword = "another", ""
				target.AccessKeyID, target.SecretAccessKey = "access", "secret"
			case "disabled with password":
				target.ThirdPartyBackupEnabled, target.RepoPassword = false, "repo-password"
			case "input without AWS":
				target.RepoPassword = "repo-password"
			case "input without access":
				target.RepoPassword, target.SecretAccessKey = "repo-password", "secret"
			case "input without secret":
				target.RepoPassword, target.AccessKeyID = "repo-password", "access"
			}
			_, err := h.secrets.Update(secret)
			require.NoError(t, err)
			client.ClearActions()
			kubeClient := h.clientset.(*kubefake.Clientset)
			kubeClient.ClearActions()
			setting, err := h.settings.Get(settings.BackupTargetSettingName, metav1.GetOptions{})
			require.NoError(t, err)
			_, err = h.syncS3Target(setting, target)
			require.Error(t, err)
			require.Equal(t, secret.Data, readCredentials(t, h).Data)
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb(), "invalid input must not change resources")
			}
			require.Empty(t, kubeClient.Actions(), "invalid input must not remove maintenance jobs")
		})
	}
}

func TestPasswordSyncRetries(t *testing.T) {
	for _, tt := range []struct{ name, resource, password string }{
		{"save Secret", "secrets", "repo-password"},
		{"mask saved credentials", "settings", "repo-password"},
		{"clear passwords", "secrets", ""},
		{"mask disabled target", "settings", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, setting, target, client := credentialHandler(t)
			if tt.password == "" {
				initRepo(t, h, target)
			}
			original := readCredentials(t, h)
			target.ThirdPartyBackupEnabled, target.RepoPassword = tt.password != "", tt.password
			target.AccessKeyID, target.SecretAccessKey = "access", "secret"
			setTargetValue(t, h, setting, target)
			_, err := h.settings.Update(setting)
			require.NoError(t, err)
			fail := true
			client.PrependReactor("update", tt.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				if tt.resource == "settings" && action.(clienttesting.UpdateAction).GetObject().(*harvesterv1.Setting).Value == setting.Value {
					return false, nil, nil
				}
				if fail {
					return true, nil, fmt.Errorf("injected write failure")
				}
				return false, nil, nil
			})
			_, err = h.syncS3Target(setting, target)
			require.ErrorContains(t, err, "injected write failure")
			stored, err := h.settings.Get(setting.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, setting.Value, stored.Value)
			if tt.resource == "secrets" {
				require.Equal(t, original.Data, readCredentials(t, h).Data)
			} else {
				assertRepoPassword(t, readCredentials(t, h), tt.password)
			}
			fail = false
			updated, err := h.OnBackupTargetChange(stored.Name, stored)
			require.NoError(t, err)
			masked, err := settings.DecodeBackupTarget(updated.Value)
			require.NoError(t, err)
			require.Empty(t, masked.AccessKeyID)
			require.Empty(t, masked.SecretAccessKey)
			require.Empty(t, masked.RepoPassword)
			assertRepoPassword(t, readCredentials(t, h), tt.password)
		})
	}
}

func TestResourceConfigPreservesCredentials(t *testing.T) {
	h, _, target, client := credentialHandler(t)
	initRepo(t, h, target)
	initial := readCredentials(t, h)
	client.ClearActions()
	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.ThirdPartyBackupConfigSettingName},
		Value:      `{"restic":{"memoryLimit":"8Gi"}}`,
	}
	updated, err := h.OnBackupTargetChange(setting.Name, setting)
	require.NoError(t, err)
	require.Equal(t, setting, updated)
	require.Empty(t, client.Actions(), "resource updates must not access the credential Secret")
	require.Equal(t, initial.Data, readCredentials(t, h).Data)
}

func initRepo(t *testing.T, h *TargetHandler, target *settings.BackupTarget) {
	t.Helper()
	target.ThirdPartyBackupEnabled, target.RepoPassword = true, "repo-password"
	target.AccessKeyID, target.SecretAccessKey = "access", "secret"
	require.NoError(t, syncTestTarget(t, h, target))
}

func setTargetValue(t *testing.T, h *TargetHandler, setting *harvesterv1.Setting, target *settings.BackupTarget) {
	t.Helper()
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting.Value = string(value)
	_, err = h.settings.Update(setting)
	require.NoError(t, err)
}

func assertRepoPassword(t *testing.T, secret *corev1.Secret, password string) {
	t.Helper()
	for _, key := range []string{resticutil.PasswordKey, kopiautil.PasswordKey} {
		if password == "" {
			require.NotContains(t, secret.Data, key)
			continue
		}
		require.Equal(t, []byte(password), secret.Data[key], key)
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
	target.ThirdPartyBackupEnabled = false
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName}, Value: string(value),
	}
	harvesterv1.SettingConfigured.True(setting)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName, UID: "source-uid", ResourceVersion: "1"},
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
	client.PrependReactor("create", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		secret := action.(clienttesting.CreateAction).GetObject().(*corev1.Secret)
		secret.UID, secret.ResourceVersion = "new-source-uid", "1"
		return false, nil, nil
	})
	h := &TargetHandler{
		ctx: context.Background(), clientset: kubefake.NewSimpleClientset(),
		initRepos:           func(context.Context, *settings.BackupTarget, map[string][]byte) error { return nil },
		connectRepos:        func(context.Context, *settings.BackupTarget, map[string][]byte) error { return nil },
		secrets:             fakeclients.SecretClient(client.CoreV1().Secrets),
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

// Keep direct submission tests on the same path as the Setting controller.
func syncTestTarget(t *testing.T, h *TargetHandler, target *settings.BackupTarget) error {
	t.Helper()
	setting, err := h.settings.Get(settings.BackupTargetSettingName, metav1.GetOptions{})
	require.NoError(t, err)
	setTargetValue(t, h, setting, target)
	copy := *target
	_, err = h.syncS3Target(setting, &copy)
	return err
}

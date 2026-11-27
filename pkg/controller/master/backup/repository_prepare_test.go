package backup

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
)

func submitRepository(t *testing.T, h *TargetHandler, setting *harvesterv1.Setting, target *settings.BackupTarget) {
	t.Helper()
	target.ThirdPartyBackupEnabled = true
	target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "new-access", "new-secret", "repo-password"
	setTargetValue(t, h, setting, target)
}

func currentTarget(t *testing.T, h *TargetHandler) *harvesterv1.Setting {
	t.Helper()
	setting, err := h.settings.Get(settings.BackupTargetSettingName, metav1.GetOptions{})
	require.NoError(t, err)
	return setting
}

func TestPreparationBeforePublication(t *testing.T) {
	for _, failure := range []string{"", "Restic failed", "Kopia failed"} {
		t.Run(failure, func(t *testing.T) {
			h, setting, target, _ := credentialHandler(t)
			submitRepository(t, h, setting, target)
			original := readCredentials(t, h)
			h.initRepos = func(_ context.Context, input *settings.BackupTarget, data map[string][]byte) error {
				require.Equal(t, target.AccessKeyID, string(data[util.AWSAccessKey]))
				require.Equal(t, target.RepoPassword, string(data[backuputil.KopiaPasswordKey]))
				require.Equal(t, original.Data, readCredentials(t, h).Data)
				require.False(t, harvesterv1.SettingConfigured.IsTrue(currentTarget(t, h)))
				require.Equal(t, setting.Value, currentTarget(t, h).Value)
				if failure != "" {
					return fmt.Errorf("%s", failure)
				}
				return nil
			}
			_, _ = h.OnBackupTargetChange(setting.Name, setting)
			current := currentTarget(t, h)
			if failure != "" {
				require.Equal(t, original.Data, readCredentials(t, h).Data)
				require.Equal(t, setting.Value, current.Value)
				require.False(t, harvesterv1.SettingConfigured.IsTrue(current))
				require.Empty(t, current.Annotations[backuputil.ReadyAnnotation])
				return
			}
			masked, err := settings.DecodeBackupTarget(current.Value)
			require.NoError(t, err)
			require.True(t, backuputil.Masked(masked))
			require.NoError(t, backuputil.CheckRepoReady(current, masked, readCredentials(t, h)))
		})
	}
}

func TestPublicationRecovery(t *testing.T) {
	for _, step := range []string{"secret", "longhorn", "mask", "status"} {
		t.Run(step, func(t *testing.T) {
			h, setting, target, client := credentialHandler(t)
			submitRepository(t, h, setting, target)
			original := readCredentials(t, h)
			calls := 0
			h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				calls++
				return nil
			}
			h.connectRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				t.Fatal("publication recovery must reuse a valid receipt after masking")
				return nil
			}
			fail := true
			client.PrependReactor("update", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
				if !fail {
					return false, nil, nil
				}
				resource := action.GetResource().Resource
				blocked := step == "secret" && resource == "secrets" || step == "longhorn" && resource == "backuptargets"
				if resource == "settings" {
					next := action.(clienttesting.UpdateAction).GetObject().(*harvesterv1.Setting)
					blocked = step == "mask" && next.Value != setting.Value ||
						step == "status" && harvesterv1.SettingConfigured.IsTrue(next)
				}
				if blocked {
					return true, nil, fmt.Errorf("injected %s failure", step)
				}
				return false, nil, nil
			})
			_, _ = h.OnBackupTargetChange(setting.Name, setting)
			pending := currentTarget(t, h)
			require.False(t, harvesterv1.SettingConfigured.IsTrue(pending))
			if step == "secret" {
				require.Equal(t, original.Data, readCredentials(t, h).Data)
			}
			if step != "status" {
				require.Equal(t, setting.Value, pending.Value)
			}
			fail = false
			_, err := h.OnBackupTargetChange(pending.Name, pending)
			require.NoError(t, err)
			ready := currentTarget(t, h)
			masked, err := settings.DecodeBackupTarget(ready.Value)
			require.NoError(t, err)
			require.NoError(t, backuputil.CheckRepoReady(ready, masked, readCredentials(t, h)))
			wantCalls := 2
			if step == "status" {
				wantCalls = 1
			}
			require.Equal(t, wantCalls, calls)
		})
	}
}

func TestSupersededPreparation(t *testing.T) {
	h, setting, target, _ := credentialHandler(t)
	submitRepository(t, h, setting, target)
	original := readCredentials(t, h)
	var newer *harvesterv1.Setting
	h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
		newer = currentTarget(t, h)
		replacement := *target
		replacement.BucketName = "newer-bucket"
		setTargetValue(t, h, newer, &replacement)
		return nil
	}
	_, err := h.OnBackupTargetChange(setting.Name, setting)
	require.True(t, apierrors.IsConflict(err), "%v", err)
	require.Equal(t, original.Data, readCredentials(t, h).Data)
	require.Equal(t, newer, currentTarget(t, h), "stale results must not update the new submission's status")
}

func TestMaskedReconnect(t *testing.T) {
	for _, interval := range []string{"", `,"refreshIntervalInSeconds":0`, `,"refreshIntervalInSeconds":null`} {
		t.Run(interval, func(t *testing.T) {
			h, _, target, _ := credentialHandler(t)
			initRepo(t, h, target)
			setting := currentTarget(t, h)
			// A masked setting with a missing receipt requires a new connection check.
			setting.Value = `{"type":"s3","endpoint":"http://minio.example.com:9000",` +
				`"bucketName":"backups","bucketRegion":"test-region","thirdPartyBackupEnabled":true` + interval + `}`
			delete(setting.Annotations, backuputil.ReadyAnnotation)
			_, err := h.settings.Update(setting)
			require.NoError(t, err)
			original := readCredentials(t, h)
			calls := 0
			h.connectRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				calls++
				return nil
			}
			h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				t.Fatal("masked reconciliation must not initialize repositories")
				return nil
			}
			_, err = h.OnBackupTargetChange(setting.Name, setting)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, original, readCredentials(t, h))
			current := currentTarget(t, h)
			_, err = h.OnBackupTargetChange(current.Name, current)
			require.NoError(t, err)
			require.Equal(t, 1, calls, "a published receipt is reusable")
		})
	}
}

func TestMaskedReceiptReuse(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			h, _, target, client := credentialHandler(t)
			initRepo(t, h, target)
			setting := currentTarget(t, h)
			if configured {
				harvesterv1.SettingConfigured.True(setting)
			}
			_, err := h.settings.Update(setting)
			require.NoError(t, err)
			target = targetFromValue(t, setting.Value)
			secret := readCredentials(t, h)
			if !configured {
				require.Error(t, backuputil.CheckRepoReady(setting, target, secret), "Jobs must still wait for configured status")
			}
			h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				t.Fatal("a valid receipt must not trigger repository commands")
				return nil
			}
			h.connectRepos = h.initRepos
			client.ClearActions()
			core := h.clientset.(*kubefake.Clientset)
			core.ClearActions()
			updated, err := h.reconcileMaskedS3(setting, target)
			require.NoError(t, err)
			require.Equal(t, setting, updated)
			require.Empty(t, core.Actions(), "receipt reuse must not stop maintenance")
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb(), "receipt reuse must not write resources")
			}
			ready, err := h.updateConfiguredStatus(updated, target)
			require.NoError(t, err)
			require.NoError(t, backuputil.CheckRepoReady(ready, target, secret))
		})
	}
}

func TestStaleReceiptReconnect(t *testing.T) {
	for _, scenario := range []string{"stale receipt", "secret changed", "connection failed"} {
		t.Run(scenario, func(t *testing.T) {
			h, _, target, client := credentialHandler(t)
			initRepo(t, h, target)
			setting := currentTarget(t, h)
			secret := readCredentials(t, h)
			if scenario == "secret changed" {
				secret.ResourceVersion = "new-version"
				secret.Data[util.AWSAccessKey] = []byte("rotated-access")
				_, err := h.secrets.Update(secret)
				require.NoError(t, err)
			} else {
				setting.Annotations[backuputil.ReadyAnnotation] = "stale"
			}
			harvesterv1.SettingConfigured.True(setting)
			_, err := h.settings.Update(setting)
			require.NoError(t, err)
			h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
				t.Fatal("masked reconciliation must not initialize repositories")
				return nil
			}
			calls := 0
			h.connectRepos = func(_ context.Context, _ *settings.BackupTarget, data map[string][]byte) error {
				calls++
				require.Equal(t, secret.Data, data)
				require.True(t, harvesterv1.SettingConfigured.IsFalse(currentTarget(t, h)))
				if scenario == "connection failed" {
					return fmt.Errorf("repository missing")
				}
				return nil
			}
			client.ClearActions()
			target = targetFromValue(t, setting.Value)
			updated, err := h.reconcileMaskedS3(setting, target)
			require.Equal(t, 1, calls)
			require.Equal(t, secret, readCredentials(t, h))
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" {
					require.True(t, action.Matches("update", "settings"), "reconnection only writes readiness")
				}
			}
			if scenario == "connection failed" {
				require.ErrorContains(t, err, "repository missing")
				require.Empty(t, currentTarget(t, h).Annotations[backuputil.ReadyAnnotation])
				return
			}
			require.NoError(t, err)
			require.Equal(t, setting.Value, updated.Value)
			require.NoError(t, backuputil.CheckRepoReceipt(updated, target, secret))
			require.Error(t, backuputil.CheckRepoReady(updated, target, secret), "status publication is a separate step")
		})
	}
}

func TestSecretChangedDuringValidation(t *testing.T) {
	h, _, target, _ := credentialHandler(t)
	initRepo(t, h, target)
	setting := currentTarget(t, h)
	delete(setting.Annotations, backuputil.ReadyAnnotation)
	_, err := h.settings.Update(setting)
	require.NoError(t, err)
	h.connectRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error {
		secret := readCredentials(t, h)
		secret.ResourceVersion = "new-revision"
		_, err := h.secrets.Update(secret)
		return err
	}
	_, err = h.syncS3Target(setting, targetFromValue(t, setting.Value))
	require.ErrorContains(t, err, "credentials changed during validation")
	require.False(t, harvesterv1.SettingConfigured.IsTrue(currentTarget(t, h)))
}

func targetFromValue(t *testing.T, value string) *settings.BackupTarget {
	t.Helper()
	target, err := settings.DecodeBackupTarget(value)
	require.NoError(t, err)
	return target
}

func TestMaintenanceStopsBeforeCredentialsChange(t *testing.T) {
	for _, action := range []string{"enable", "disable", "default", "nfs"} {
		t.Run(action, func(t *testing.T) {
			h, setting, target, _ := credentialHandler(t)
			original := readCredentials(t, h)
			submitRepository(t, h, setting, target)
			switch action {
			case "disable":
				target.ThirdPartyBackupEnabled, target.RepoPassword = false, ""
			case "default":
				target = &settings.BackupTarget{}
			case "nfs":
				target = &settings.BackupTarget{Type: settings.NFSBackupType, Endpoint: "nfs:/backup"}
			}
			setTargetValue(t, h, setting, target)
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
				Name: "maintenance", Namespace: util.LonghornSystemNamespaceName, UID: "job",
				OwnerReferences: []metav1.OwnerReference{{Kind: "CronJob", Name: kopiautil.QuickMaintenanceCronJobName, Controller: new(true)}},
			}}
			_, err := h.clientset.BatchV1().Jobs(job.Namespace).Create(h.ctx, job, metav1.CreateOptions{})
			require.NoError(t, err)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "maintenance", Namespace: job.Namespace, Labels: map[string]string{kopiautil.LabelKopiaMaintenance: "true"},
			}}
			_, err = h.clientset.CoreV1().Pods(pod.Namespace).Create(h.ctx, pod, metav1.CreateOptions{})
			require.NoError(t, err)
			calls := 0
			h.initRepos = func(context.Context, *settings.BackupTarget, map[string][]byte) error { calls++; return nil }
			_, err = h.syncTarget(setting, target)
			require.ErrorContains(t, err, "waiting for Kopia maintenance")
			require.Zero(t, calls)
			require.Equal(t, original.Data, readCredentials(t, h).Data)
			// Job deletion alone does not authorize a credential change.
			_, err = h.syncTarget(currentTarget(t, h), target)
			require.ErrorContains(t, err, "waiting for Kopia maintenance")
			require.NoError(t, h.clientset.CoreV1().Pods(pod.Namespace).Delete(h.ctx, pod.Name, metav1.DeleteOptions{}))
			_, err = h.syncTarget(currentTarget(t, h), target)
			require.NoError(t, err)
		})
	}
}

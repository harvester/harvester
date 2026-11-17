package setting

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestTargetPasswordAdmission(t *testing.T) {
	for _, state := range []string{
		"matching", "no secret", "no ID", "empty passwords", "new repository",
		"empty input", "blank input", "nul input", "replacement", "missing restic", "missing kopia",
		"inconsistent passwords", "enabled nfs", "read failure",
	} {
		t.Run(state, func(t *testing.T) {
			target := passwordTarget()
			secret := thirdPartySecret(target)
			valid := false
			switch state {
			case "matching", "no secret":
				valid = true
			case "no ID":
				delete(secret.Data, backuputil.RepositoryIDKey)
				valid = true
			case "empty passwords":
				delete(secret.Data, backuputil.ResticPasswordKey)
				delete(secret.Data, backuputil.KopiaPasswordKey)
				valid = true
			case "new repository":
				target.BucketName, target.RepoPassword = "another", "new-password"
				valid = true
			case "empty input":
				target.RepoPassword = ""
			case "blank input":
				target.RepoPassword = " \t"
			case "nul input":
				target.RepoPassword = "bad\x00password"
			case "replacement":
				target.RepoPassword = "different"
			case "missing restic":
				delete(secret.Data, backuputil.ResticPasswordKey)
			case "missing kopia":
				delete(secret.Data, backuputil.KopiaPasswordKey)
			case "inconsistent passwords":
				secret.Data[backuputil.KopiaPasswordKey] = []byte("different")
			case "enabled nfs":
				target.Type = settings.NFSBackupType
			}
			var objects []runtime.Object
			if state != "no secret" {
				objects = append(objects, secret)
			}
			validator, client := thirdPartyValidator(objects...)
			if state == "read failure" {
				client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, fmt.Errorf("injected read failure")
				})
			}
			err := validator.checkThirdPartyInput(target)
			if valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestDisabledPasswordAdmission(t *testing.T) {
	for _, targetType := range []settings.TargetType{"", settings.S3BackupType, settings.NFSBackupType} {
		for _, password := range []string{"", "repo-password", " "} {
			t.Run(fmt.Sprintf("type=%s/password=%q", targetType, password), func(t *testing.T) {
				validator, client := thirdPartyValidator()
				target := &settings.BackupTarget{Type: targetType, RepoPassword: password}
				err := validator.validateBackupTarget(target)
				switch {
				case password != "":
					require.ErrorContains(t, err, "requires third-party backups to be enabled")
				case targetType == settings.S3BackupType:
					require.ErrorContains(t, err, "should have access key and access key id")
				case targetType == settings.NFSBackupType:
					require.ErrorContains(t, err, "should have endpoint")
				default:
					require.NoError(t, err)
				}
				for _, action := range client.Actions() {
					require.NotEqual(t, "secrets", action.GetResource().Resource,
						"disabling and resetting must not validate stored passwords")
				}
			})
		}
	}
}

func TestTargetPasswordCleanup(t *testing.T) {
	for _, state := range []string{"controller", "user", "controller target change", "metadata only"} {
		t.Run(state, func(t *testing.T) {
			busy := &harvesterv1.VirtualMachineBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "busy", Namespace: "default"},
				Spec:       harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
			}
			validator, client := thirdPartyValidator(busy)
			client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("cleanup must not read the Secret")
			})
			target := passwordTarget()
			target.AccessKeyID, target.SecretAccessKey = "access", "secret"
			oldSetting := targetSetting(t, target)
			target.AccessKeyID, target.SecretAccessKey, target.RepoPassword = "", "", ""
			if state == "controller target change" {
				target.BucketName = "another"
			}
			updated := targetSetting(t, target)
			if state == "metadata only" {
				updated = oldSetting.DeepCopy()
				updated.Annotations = map[string]string{"test": "metadata"}
			}
			err := validator.Update(targetRequest(state != "user"), oldSetting, updated)
			if state == "controller" || state == "metadata only" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "thirdPartyRepositoryPassword is required")
		})
	}
}

func TestTargetPasswordUpdates(t *testing.T) {
	for _, state := range []string{
		"enable", "disable", "resource change", "missing password", "legacy LH", "access rotation", "secret rotation",
	} {
		t.Run(state, func(t *testing.T) {
			target := passwordTarget()
			secret := thirdPartySecret(target)
			target.RepoPassword = ""
			target.ThirdPartyBackupEnabled = state != "enable" && state != "legacy LH"
			old := targetSetting(t, target)
			validator, _ := thirdPartyValidator(old, secret)
			target.RepoPassword = "repo-password"
			switch state {
			case "enable":
				target.ThirdPartyBackupEnabled = true
			case "disable":
				target.ThirdPartyBackupEnabled, target.RepoPassword = false, ""
			case "resource change":
				target.RefreshIntervalInSeconds = 300
			case "missing password", "legacy LH":
				target.RefreshIntervalInSeconds, target.RepoPassword = 300, ""
			case "access rotation":
				target.AccessKeyID = "new-access"
			case "secret rotation":
				target.SecretAccessKey = "new-secret"
			}
			err := validator.Update(targetRequest(false), old, targetSetting(t, target))
			if state == "missing password" {
				require.ErrorContains(t, err, "thirdPartyRepositoryPassword is required")
				return
			}
			require.ErrorContains(t, err, "S3 backup target should have access key and access key id")
		})
	}
}

func TestTargetChangesWhileBusy(t *testing.T) {
	for _, state := range []string{"disable", "reset", "empty value", "password", "nfs"} {
		for _, operation := range []string{"backup", "restore"} {
			t.Run(state+"/"+operation, func(t *testing.T) {
				target := passwordTarget()
				secret := thirdPartySecret(target)
				target.RepoPassword = ""
				old := targetSetting(t, target)
				var busy runtime.Object = &harvesterv1.VirtualMachineBackup{
					Spec: harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
				}
				if operation == "restore" {
					busy = &harvesterv1.VirtualMachineRestore{}
				}
				validator, _ := thirdPartyValidator(old, secret, busy)
				switch state {
				case "disable":
					target.ThirdPartyBackupEnabled = false
				case "reset":
					target = &settings.BackupTarget{}
				case "password":
					target.RepoPassword = "repo-password"
				case "nfs":
					target = &settings.BackupTarget{Type: settings.NFSBackupType, Endpoint: "nfs:/backups"}
				}
				updated := targetSetting(t, target)
				if state == "empty value" {
					updated.Value = ""
				}
				err := validator.Update(targetRequest(false), old, updated)
				require.ErrorContains(t, err, "creating or deleting")
			})
		}
	}
}

func TestPasswordDefaults(t *testing.T) {
	validator, _ := thirdPartyValidator()
	setting := targetSetting(t, passwordTarget())
	setting.Default, setting.Value = setting.Value, ""
	require.ErrorContains(t, validator.Create(nil, setting), "only be submitted in value")
	setting.Default = settings.InitBackupTargetToString()
	require.NoError(t, validator.Create(nil, setting))
}

func TestLegacyTargetCleanup(t *testing.T) {
	busy := &harvesterv1.VirtualMachineBackup{
		Spec: harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Backup},
	}
	target := passwordTarget()
	target.ThirdPartyBackupEnabled, target.RepoPassword = false, ""
	target.AccessKeyID, target.SecretAccessKey = "access", "secret"
	old := targetSetting(t, target)
	validator, _ := thirdPartyValidator(old, busy)
	target.AccessKeyID, target.SecretAccessKey = "", ""
	updated := targetSetting(t, target)
	require.NoError(t, validator.Update(targetRequest(true), old, updated))
	require.ErrorContains(t, validator.Update(targetRequest(false), old, updated), "creating or deleting")
}

func passwordTarget() *settings.BackupTarget {
	return &settings.BackupTarget{
		Type: settings.S3BackupType, Endpoint: "https://s3.example.com",
		BucketName: "backups", BucketRegion: "test-region",
		ThirdPartyBackupEnabled: true, RepoPassword: "repo-password",
	}
}

func targetSetting(t *testing.T, target *settings.BackupTarget) *harvesterv1.Setting {
	t.Helper()
	value, err := json.Marshal(target)
	require.NoError(t, err)
	return &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName}, Value: string(value),
	}
}

func targetRequest(controller bool) *types.Request {
	const controllerName = "system:serviceaccount:harvester-system:harvester"
	username := "test-user"
	if controller {
		username = controllerName
	}
	return types.NewRequest(&webhook.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authenticationv1.UserInfo{Username: username}},
	}, &config.Options{HarvesterControllerUsername: controllerName})
}

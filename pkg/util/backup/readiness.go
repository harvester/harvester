package backup

import (
	"encoding/json"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const ReadyAnnotation = "backup.harvesterhci.io/repository-ready"

// A receipt contains public configuration and API revisions, never credentials
// or password hashes. Secret replacement or modification invalidates it.
type receipt struct {
	Target  string    `json:"target"`
	UID     types.UID `json:"secretUID"`
	Version string    `json:"secretVersion"`
}

func Masked(target *settings.BackupTarget) bool {
	return target.AccessKeyID == "" && target.SecretAccessKey == "" && target.RepoPassword == ""
}

func RepoReadyRecord(target *settings.BackupTarget, secret *corev1.Secret) (string, error) {
	if secret == nil || secret.UID == "" || secret.ResourceVersion == "" {
		return "", fmt.Errorf("backup target Secret has no persisted identity")
	}
	public := *target
	public.AccessKeyID, public.SecretAccessKey, public.RepoPassword = "", "", ""
	data, err := json.Marshal(public)
	if err != nil {
		return "", err
	}
	data, err = json.Marshal(receipt{Target: string(data), UID: secret.UID, Version: secret.ResourceVersion})
	return string(data), err
}

func CheckRepoReady(setting *harvesterv1.Setting, target *settings.BackupTarget, secret *corev1.Secret) error {
	if setting.DeletionTimestamp != nil || !Masked(target) || !harvesterv1.SettingConfigured.IsTrue(setting) {
		return fmt.Errorf("backup target repository preparation is pending")
	}
	return CheckRepoReceipt(setting, target, secret)
}

// CheckRepoReceipt validates the recorded configuration and Secret revision.
// Controllers can reuse it while completing the configured status update.
func CheckRepoReceipt(setting *harvesterv1.Setting, target *settings.BackupTarget, secret *corev1.Secret) error {
	record, err := RepoReadyRecord(target, secret)
	if err != nil {
		return err
	}
	if setting.Annotations[ReadyAnnotation] != record {
		return fmt.Errorf("backup target repositories have not been validated for the current configuration and credentials")
	}
	return nil
}

// TargetClient reads both objects live. A cached configured condition cannot
// authorize a new operation while a user submission is awaiting reconciliation.
type TargetClient struct {
	Settings ctlharvesterv1.SettingClient
	Secrets  ctlcorev1.SecretClient
}

func (c TargetClient) Get() (*harvesterv1.Setting, *settings.BackupTarget, error) {
	if c.Settings == nil {
		return nil, nil, fmt.Errorf("backup target client is unavailable")
	}
	setting, err := c.Settings.Get(settings.BackupTargetSettingName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	target, err := settings.DecodeBackupTarget(setting.Value)
	return setting, target, err
}

func (c TargetClient) ReadyRepoSecret(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*corev1.Secret, error) {
	if c.Secrets == nil {
		return nil, fmt.Errorf("backup target Secret client is unavailable")
	}
	secret, err := c.Secrets.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if err := CheckRepoReady(setting, target, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// ThirdPartyCredentials validates the recorded target and repository readiness
// before returning AWS credentials and the selected Restic or Kopia password.
func (c TargetClient) ThirdPartyCredentials(
	status *harvesterv1.BackupTarget, key string,
) (map[string][]byte, error) {
	if _, err := TargetFromStatus(status); err != nil {
		return nil, err
	}
	setting, target, err := c.Get()
	if err != nil {
		return nil, err
	}
	if !IsBackupTargetSame(status, target) {
		return nil, fmt.Errorf("VMBackup backup target does not match the configured backup target")
	}
	if !target.ThirdPartyBackupEnabled {
		return nil, fmt.Errorf("third-party backups are disabled")
	}
	if err := c.CheckThirdPartyOptions(target); err != nil {
		return nil, err
	}
	secret, err := c.ReadyRepoSecret(setting, target)
	if err != nil {
		return nil, err
	}
	return selectCredentials(secret.Data, status.RepositoryID, key)
}

// CheckLonghornReady gates new native Longhorn backups only when the shared
// target enables third-party backups. Longhorn-only installations need no receipt.
func (c TargetClient) CheckLonghornReady() error {
	setting, target, err := c.Get()
	if err != nil {
		return err
	}
	if !target.ThirdPartyBackupEnabled {
		return nil
	}
	_, err = c.ReadyRepoSecret(setting, target)
	return err
}

package backup

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

func (h *TargetHandler) prepareTarget(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	secret, data, err := h.prepareRepoSecret(target)
	if err != nil {
		return setting, err
	}
	setting, err = h.clearRepoReadiness(setting)
	if err != nil {
		return setting, err
	}
	if err := h.stopMaintenance(); err != nil {
		return setting, err
	}
	if err := h.initRepos(h.ctx, target, data); err != nil {
		return setting, err
	}
	setting, err = h.currentSetting(setting)
	if err != nil {
		return setting, err
	}
	if err := h.saveS3Secret(secret, data, target); err != nil {
		return setting, err
	}
	if err := h.syncLonghornS3(target); err != nil {
		return setting, err
	}
	saved, err := h.getBackupSecret()
	if err != nil {
		return setting, err
	}
	if saved == nil || !reflect.DeepEqual(saved.Data, data) {
		return setting, fmt.Errorf("backup target credentials changed during preparation")
	}
	updated, err := h.prepareReceipt(setting, target, saved)
	if err != nil {
		return setting, err
	}
	// Publish readiness and mask submitted credentials in the same Setting update.
	return h.stripCredentials(updated, target)
}

// prepareRepoSecret validates submitted AWS and repository credentials and builds
// candidate Secret data. It does not write the shared Secret.
func (h *TargetHandler) prepareRepoSecret(
	target *settings.BackupTarget,
) (*corev1.Secret, map[string][]byte, error) {
	if err := backuputil.CheckPasswordInput(target.RepoPassword); err != nil {
		return nil, nil, err
	}
	secret, data, err := h.prepareS3Secret(target)
	if err != nil {
		return nil, nil, err
	}
	if err := backuputil.CheckPasswordChange(data, backuputil.RepositoryID(target), target.RepoPassword); err != nil {
		return nil, nil, err
	}
	data[backuputil.ResticPasswordKey] = []byte(target.RepoPassword)
	data[backuputil.KopiaPasswordKey] = []byte(target.RepoPassword)
	return secret, data, nil
}

// refreshRepoReceipt reconnects with saved credentials and records the result.
// Masking already completed Secret publication and Longhorn synchronization.
func (h *TargetHandler) refreshRepoReceipt(
	setting *harvesterv1.Setting, target *settings.BackupTarget, secret *corev1.Secret,
) (*harvesterv1.Setting, error) {
	setting, err := h.clearRepoReadiness(setting)
	if err != nil {
		return setting, err
	}
	if err := h.stopMaintenance(); err != nil {
		return setting, err
	}
	if err := h.connectSavedRepos(target, secret); err != nil {
		return setting, err
	}
	updated, err := h.prepareReceipt(setting, target, secret)
	if err != nil {
		return setting, err
	}
	saved, err := h.settings.Update(updated)
	if err != nil {
		return setting, err
	}
	return saved, nil
}

func (h *TargetHandler) connectSavedRepos(target *settings.BackupTarget, secret *corev1.Secret) error {
	if err := h.connectRepos(h.ctx, target, secret.Data); err != nil {
		return err
	}
	current, err := h.getBackupSecret()
	if err != nil {
		return err
	}
	if current == nil || current.UID != secret.UID || current.ResourceVersion != secret.ResourceVersion {
		return fmt.Errorf("backup target credentials changed during validation")
	}
	return nil
}

func (h *TargetHandler) clearRepoReadiness(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	current, err := h.currentSetting(setting)
	if err != nil {
		return setting, err
	}
	updated := current.DeepCopy()
	delete(updated.Annotations, backuputil.ReadyAnnotation)
	if !harvesterv1.SettingConfigured.IsFalse(updated) {
		harvesterv1.SettingConfigured.False(updated)
		harvesterv1.SettingConfigured.Message(updated, "Validating backup repositories")
	}
	if reflect.DeepEqual(current, updated) {
		return current, nil
	}
	saved, err := h.settings.Update(updated)
	if err != nil {
		return setting, err
	}
	return saved, nil
}

// currentSetting rejects results from a superseded submission. Metadata-only
// writes can be retained; every subsequent update still uses API conflict checks.
func (h *TargetHandler) currentSetting(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	current, err := h.settings.Get(setting.Name, metav1.GetOptions{})
	if err != nil {
		return setting, err
	}
	if current.DeletionTimestamp != nil || current.UID != setting.UID || current.Value != setting.Value {
		return setting, apierrors.NewConflict(schema.GroupResource{
			Group: harvesterv1.SchemeGroupVersion.Group, Resource: "settings",
		}, setting.Name, fmt.Errorf("backup target changed during preparation"))
	}
	return current, nil
}

// prepareReceipt rechecks the current Setting and adds readiness to a copy.
// The caller persists it; configured remains false until target sync completes.
func (h *TargetHandler) prepareReceipt(
	setting *harvesterv1.Setting, target *settings.BackupTarget, secret *corev1.Secret,
) (*harvesterv1.Setting, error) {
	current, err := h.currentSetting(setting)
	if err != nil {
		return setting, err
	}
	record, err := backuputil.RepoReadyRecord(target, secret)
	if err != nil {
		return setting, err
	}
	updated := current.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	updated.Annotations[backuputil.ReadyAnnotation] = record
	return updated, nil
}

func (h *TargetHandler) clearDisabledReceipt(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	if target.ThirdPartyBackupEnabled || setting.Annotations[backuputil.ReadyAnnotation] == "" {
		return setting, nil
	}
	updated := setting.DeepCopy()
	delete(updated.Annotations, backuputil.ReadyAnnotation)
	saved, err := h.settings.Update(updated)
	if err != nil {
		return setting, err
	}
	return saved, nil
}

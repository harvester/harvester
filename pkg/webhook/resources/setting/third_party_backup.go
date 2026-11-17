package setting

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func decodeBackupConfig(setting *harvesterv1.Setting) (*settings.ThirdPartyBackupConfig, error) {
	_, err := settings.DecodeThirdPartyBackupConfig(setting.Default)
	if err != nil {
		return nil, werror.NewInvalidError(err.Error(), settings.KeywordDefault)
	}
	config, err := settings.DecodeThirdPartyBackupConfig(setting.Value)
	if err != nil {
		return nil, werror.NewInvalidError(err.Error(), settings.KeywordValue)
	}
	return config, nil
}

func (v *settingValidator) checkConfigCreate(setting *harvesterv1.Setting) error {
	_, err := decodeBackupConfig(setting)
	// The settings controller creates the default Setting with no user input.
	if err != nil || setting.Value == "" {
		return err
	}
	return v.checkThirdPartyIdle()
}

func (v *settingValidator) checkConfigUpdate(
	_ *types.Request, oldSetting, newSetting *harvesterv1.Setting,
) error {
	_, err := decodeBackupConfig(newSetting)
	if err != nil {
		return err
	}
	if oldSetting.Value == newSetting.Value && oldSetting.Default == newSetting.Default {
		return nil
	}
	return v.checkThirdPartyIdle()
}

func (v *settingValidator) checkConfigDelete(_ *harvesterv1.Setting) error {
	return v.checkThirdPartyIdle()
}

func (v *settingValidator) checkThirdPartyIdle() error {
	if err := v.checkThirdPartyBackups(); err != nil {
		return err
	}
	return v.checkThirdPartyRestores()
}

func (v *settingValidator) checkThirdPartyBackups() error {
	backups, err := v.vmBackupCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Can't list VM backups: %v", err))
	}
	for _, backup := range backups {
		if !v.vmbr.GetType(backup).IsDataMover() {
			continue
		}
		if v.vmbr.GetDeletionTimestamp(backup) != nil || !v.vmbr.IsReady(backup) {
			return werror.NewBadRequest(fmt.Sprintf(
				"Can't change third-party-backup-config while VMBackup %s/%s is creating or deleting",
				backup.Namespace, backup.Name))
		}
	}
	return nil
}

func (v *settingValidator) checkThirdPartyRestores() error {
	restores, err := v.vmRestoreCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Can't list VM restores: %v", err))
	}
	for _, restore := range restores {
		if !v.vmrr.IsDeleting(restore) && v.vmrr.IsComplete(restore) {
			continue
		}
		backupType, err := v.restoreBackupType(restore)
		if err != nil {
			return err
		}
		if backupType.IsDataMover() {
			return werror.NewBadRequest(fmt.Sprintf(
				"Can't change third-party-backup-config while VMRestore %s/%s is creating or deleting",
				restore.Namespace, restore.Name))
		}
	}
	return nil
}

func (v *settingValidator) restoreBackupType(
	restore *harvesterv1.VirtualMachineRestore,
) (harvesterv1.BackupType, error) {
	if backupType := v.vmrr.GetType(restore); backupType != "" {
		return backupType, nil
	}
	// New restores have no engine annotation until their controller processes them.
	backup, err := v.vmBackupCache.Get(v.vmrr.GetVMBackupNamespace(restore), v.vmrr.GetVMBackupName(restore))
	if err != nil {
		return "", werror.NewInternalError(fmt.Sprintf("Can't identify backup type for VMRestore %s/%s: %v",
			restore.Namespace, restore.Name, err))
	}
	return v.vmbr.GetType(backup), nil
}

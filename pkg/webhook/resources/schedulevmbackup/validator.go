package schedulevmbackup

import (
	"fmt"
	"os"

	"github.com/longhorn/backupstore"
	ctlv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/robfig/cron"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldCron          = "spec.cron"
	fieldMaxFailure    = "spec.maxFailure"
	fieldResumeRequest = "spec.resumeRequest"
)

type scheuldeVMBackupValidator struct {
	types.DefaultValidator
	settingCache ctlharvesterv1.SettingCache
	secretCache  ctlv1.SecretCache
}

func NewValidator(
	settingCache ctlharvesterv1.SettingCache,
	secretCache ctlv1.SecretCache,
) types.Validator {
	return &scheuldeVMBackupValidator{
		settingCache: settingCache,
		secretCache:  secretCache,
	}
}

func (v *scheuldeVMBackupValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.ScheduleVMBackupResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.ScheduleVMBackup{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func (v *scheuldeVMBackupValidator) checkTargetHealth() error {
	targetSetting, err := v.settingCache.Get(settings.BackupTargetSettingName)
	if err != nil {
		return err
	}

	if targetSetting.Value == "" {
		return fmt.Errorf("setting %s is empty", settings.BackupTargetSettingName)
	}

	target, err := settings.DecodeBackupTarget(targetSetting.Value)
	if err != nil {
		return fmt.Errorf("can't decode setting %s value %s, error: %w", settings.BackupTargetSettingName, targetSetting.Value, err)
	}

	if target.IsDefaultBackupTarget() {
		return fmt.Errorf("setting %s is not set", settings.BackupTargetSettingName)
	}

	if target.Type == settings.S3BackupType {
		secret, err := v.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
		if err != nil {
			return fmt.Errorf("can't get backup target secret: %s/%s, error: %w",
				util.LonghornSystemNamespaceName, util.BackupTargetSecretName, err)
		}
		os.Setenv(backup.AWSAccessKey, string(secret.Data[backup.AWSAccessKey]))
		os.Setenv(backup.AWSSecretKey, string(secret.Data[backup.AWSSecretKey]))
		os.Setenv(backup.AWSEndpoints, string(secret.Data[backup.AWSEndpoints]))
		os.Setenv(backup.AWSCERT, string(secret.Data[backup.AWSCERT]))
	}

	_, err = backupstore.GetBackupStoreDriver(backup.ConstructEndpoint(target))
	if err != nil {
		return fmt.Errorf("can't connect to backup target %+v, error: %w", target, err)
	}

	return nil
}

func (v *scheuldeVMBackupValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newSVMBackup := newObj.(*v1beta1.ScheduleVMBackup)

	if newSVMBackup.Spec.MaxFailure >= newSVMBackup.Spec.Retain {
		return werror.NewInvalidError("max failure should be less than retain", fieldMaxFailure)
	}

	if _, err := cron.ParseStandard(newSVMBackup.Spec.Cron); err != nil {
		return werror.NewInvalidError("invalid cron format", fieldCron)
	}

	if err := v.checkTargetHealth(); err != nil {
		return werror.NewInvalidError(err.Error(), fieldResumeRequest)
	}

	return nil
}

func (v *scheuldeVMBackupValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newSVMBackup := newObj.(*v1beta1.ScheduleVMBackup)
	oldSVMBackup := oldObj.(*v1beta1.ScheduleVMBackup)

	if !newSVMBackup.DeletionTimestamp.IsZero() {
		return nil
	}

	if newSVMBackup.Spec.MaxFailure >= newSVMBackup.Spec.Retain {
		return werror.NewInvalidError("max failure should be less than retain", fieldMaxFailure)
	}

	if _, err := cron.ParseStandard(newSVMBackup.Spec.Cron); err != nil {
		return werror.NewInvalidError("invalid cron format", fieldCron)
	}

	if !oldSVMBackup.Spec.ResumeRequest && newSVMBackup.Spec.ResumeRequest {
		if err := v.checkTargetHealth(); err != nil {
			return werror.NewInvalidError(err.Error(), fieldResumeRequest)
		}
	}

	return nil
}

func (v *scheuldeVMBackupValidator) Delete(_ *types.Request, _ runtime.Object) error {
	return nil
}

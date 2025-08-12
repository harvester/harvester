package schedulevmbackup

import (
	"fmt"
	"reflect"
	"time"

	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/robfig/cron"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldCron       = "spec.cron"
	fieldMaxFailure = "spec.maxFailure"
	fieldSuspend    = "spec.suspend"
	fieldVMBackup   = "spec.vmbackup"

	minCronGranularity = time.Hour
	minCronOffset      = 10 * time.Minute
)

type scheuldeVMBackupValidator struct {
	types.DefaultValidator
	settingCache   ctlharvesterv1.SettingCache
	secretCache    ctlv1.SecretCache
	svmbackupCache ctlharvesterv1.ScheduleVMBackupCache
}

func NewValidator(
	settingCache ctlharvesterv1.SettingCache,
	secretCache ctlv1.SecretCache,
	svmbackupCache ctlharvesterv1.ScheduleVMBackupCache,
) types.Validator {
	return &scheuldeVMBackupValidator{
		settingCache:   settingCache,
		secretCache:    secretCache,
		svmbackupCache: svmbackupCache,
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

	if _, err := backuputil.GetBackupStoreDriver(v.secretCache, target); err != nil {
		return err
	}

	return nil
}

func cronGranularityCheck(v *scheuldeVMBackupValidator, svmbackup *v1beta1.ScheduleVMBackup) error {
	granularity, err := util.GetCronGranularity(svmbackup)
	if err != nil {
		return werror.NewInvalidError("invalid cron format", fieldCron)
	}

	if granularity < minCronGranularity {
		return fmt.Errorf("schedule granularity %s less than %s", granularity.String(), minCronGranularity.String())
	}

	svmbackups, err := v.svmbackupCache.GetByIndex(indexeres.ScheduleVMBackupByCronGranularity, granularity.String())
	if err != nil {
		return err
	}

	for _, s := range svmbackups {
		// skip check with svmbackup itself
		if s.UID == svmbackup.UID {
			continue
		}

		offset, err := util.CalculateCronOffset(svmbackup, s)
		if err != nil {
			return err
		}

		if offset < minCronOffset {
			return fmt.Errorf("same granularity as schedule %s and offset less than %s", s.Name, minCronOffset.String())
		}
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

	if !util.SkipCronGranularityCheck(newSVMBackup) {
		if err := cronGranularityCheck(v, newSVMBackup); err != nil {
			return werror.NewInvalidError(err.Error(), fieldCron)
		}
	}

	srcVM := fmt.Sprintf("%s/%s", newSVMBackup.Namespace, newSVMBackup.Spec.VMBackupSpec.Source.Name)
	svmbackups, err := v.svmbackupCache.GetByIndex(indexeres.ScheduleVMBackupBySourceVM, srcVM)
	if err != nil {
		return err
	}

	if len(svmbackups) != 0 {
		//we should only find one existing schedule
		msg := fmt.Sprintf("VM %s already has %s schedule", srcVM, svmbackups[0].Spec.VMBackupSpec.Type)
		return werror.NewInvalidError(msg, fieldVMBackup)
	}

	if newSVMBackup.Spec.VMBackupSpec.Type == v1beta1.Snapshot {
		return nil
	}

	if err := v.checkTargetHealth(); err != nil {
		return werror.NewInvalidError(err.Error(), fieldSuspend)
	}

	return nil
}

func (v *scheuldeVMBackupValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newSVMBackup := newObj.(*v1beta1.ScheduleVMBackup)
	oldSVMBackup := oldObj.(*v1beta1.ScheduleVMBackup)

	if !newSVMBackup.DeletionTimestamp.IsZero() {
		return nil
	}

	if !reflect.DeepEqual(oldSVMBackup.Spec.VMBackupSpec, newSVMBackup.Spec.VMBackupSpec) {
		return werror.NewInvalidError("source vm can't be changed", fieldVMBackup)
	}

	if newSVMBackup.Spec.MaxFailure >= newSVMBackup.Spec.Retain {
		return werror.NewInvalidError("max failure should be less than retain", fieldMaxFailure)
	}

	if _, err := cron.ParseStandard(newSVMBackup.Spec.Cron); err != nil {
		return werror.NewInvalidError("invalid cron format", fieldCron)
	}

	if !util.SkipCronGranularityCheck(newSVMBackup) {
		if err := cronGranularityCheck(v, newSVMBackup); err != nil {
			return werror.NewInvalidError(err.Error(), fieldCron)
		}
	}

	//not updated to resume schedule
	if !oldSVMBackup.Spec.Suspend || newSVMBackup.Spec.Suspend {
		return nil
	}

	if newSVMBackup.Spec.VMBackupSpec.Type == v1beta1.Snapshot {
		return nil
	}

	if err := v.checkTargetHealth(); err != nil {
		return werror.NewInvalidError(err.Error(), fieldSuspend)
	}

	return nil
}

func (v *scheuldeVMBackupValidator) Delete(_ *types.Request, _ runtime.Object) error {
	return nil
}

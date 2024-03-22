package schedulevmbackup

import (
	"context"

	catalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"

	"github.com/harvester/harvester/pkg/config"
	ctlharvbatchv1 "github.com/harvester/harvester/pkg/generated/controllers/batch/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
)

const (
	scheduleVMBackupControllerName = "schedule-vm-bakcup-controller"
	cronJobControllerName          = "cron-job-controller"
	vmBackupControllerName         = "vm-backup-controller"
)

type svmbackupHandler struct {
	svmbackupController ctlharvesterv1.ScheduleVMBackupController
	svmbackupClient     ctlharvesterv1.ScheduleVMBackupClient
	svmbackupCache      ctlharvesterv1.ScheduleVMBackupCache
	cronJobsClient      ctlharvbatchv1.CronJobClient
	cronJobCache        ctlharvbatchv1.CronJobCache
	vmBackupClient      ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache       ctlharvesterv1.VirtualMachineBackupCache
	snapshotCache       ctlsnapshotv1.VolumeSnapshotCache
	lhsnapshotClient    ctllonghornv2.SnapshotClient
	lhsnapshotCache     ctllonghornv2.SnapshotCache
	settingCache        ctlharvesterv1.SettingCache
	secretCache         ctlcorev1.SecretCache
	namespace           string
	appCache            catalogv1.AppCache
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	svmbackups := management.HarvesterFactory.Harvesterhci().V1beta1().ScheduleVMBackup()
	cronJobs := management.HarvesterBatchFactory.Batch().V1().CronJob()
	appCache := management.CatalogFactory.Catalog().V1().App().Cache()
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	snapshots := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	lhsnapshots := management.LonghornFactory.Longhorn().V1beta2().Snapshot()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()

	svmbackupHandler := &svmbackupHandler{
		svmbackupController: svmbackups,
		svmbackupClient:     svmbackups,
		svmbackupCache:      svmbackups.Cache(),
		cronJobsClient:      cronJobs,
		cronJobCache:        cronJobs.Cache(),
		vmBackupClient:      vmBackups,
		vmBackupCache:       vmBackups.Cache(),
		snapshotCache:       snapshots.Cache(),
		lhsnapshotClient:    lhsnapshots,
		lhsnapshotCache:     lhsnapshots.Cache(),
		settingCache:        settings.Cache(),
		secretCache:         secrets.Cache(),
		namespace:           options.Namespace,
		appCache:            appCache,
	}

	svmbackups.OnChange(ctx, scheduleVMBackupControllerName, svmbackupHandler.OnChanged)
	svmbackups.OnRemove(ctx, scheduleVMBackupControllerName, svmbackupHandler.OnRemove)
	cronJobs.OnChange(ctx, cronJobControllerName, svmbackupHandler.OnCronjobChanged)
	vmBackups.OnChange(ctx, vmBackupControllerName, svmbackupHandler.OnVMBackupChange)
	vmBackups.OnRemove(ctx, vmBackupControllerName, svmbackupHandler.OnVMBackupRemove)
	return nil
}

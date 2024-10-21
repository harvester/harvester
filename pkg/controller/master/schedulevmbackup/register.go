package schedulevmbackup

import (
	"context"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
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
	longhornBackupControllerName   = "longhorn-backup-controller"

	vmBackupKindName = "VirtualMachineBackup"
)

var vmBackupKind = harvesterv1.SchemeGroupVersion.WithKind(vmBackupKindName)

type svmbackupHandler struct {
	svmbackupController  ctlharvesterv1.ScheduleVMBackupController
	svmbackupClient      ctlharvesterv1.ScheduleVMBackupClient
	svmbackupCache       ctlharvesterv1.ScheduleVMBackupCache
	cronJobsClient       ctlharvbatchv1.CronJobClient
	cronJobCache         ctlharvbatchv1.CronJobCache
	vmBackupController   ctlharvesterv1.VirtualMachineBackupController
	vmBackupClient       ctlharvesterv1.VirtualMachineBackupClient
	vmBackupCache        ctlharvesterv1.VirtualMachineBackupCache
	snapshotCache        ctlsnapshotv1.VolumeSnapshotCache
	settingController    ctlharvesterv1.SettingController
	lhbackupCache        ctllonghornv2.BackupCache
	lhbackupClient       ctllonghornv2.BackupClient
	snapshotContentCache ctlsnapshotv1.VolumeSnapshotContentCache
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	svmbackups := management.HarvesterFactory.Harvesterhci().V1beta1().ScheduleVMBackup()
	cronJobs := management.HarvesterBatchFactory.Batch().V1().CronJob()
	vmBackups := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	snapshots := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	lhbackups := management.LonghornFactory.Longhorn().V1beta2().Backup()
	snapshotContents := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent()

	svmbackupHandler := &svmbackupHandler{
		svmbackupController:  svmbackups,
		svmbackupClient:      svmbackups,
		svmbackupCache:       svmbackups.Cache(),
		cronJobsClient:       cronJobs,
		cronJobCache:         cronJobs.Cache(),
		vmBackupController:   vmBackups,
		vmBackupClient:       vmBackups,
		vmBackupCache:        vmBackups.Cache(),
		snapshotCache:        snapshots.Cache(),
		settingController:    settings,
		lhbackupCache:        lhbackups.Cache(),
		lhbackupClient:       lhbackups,
		snapshotContentCache: snapshotContents.Cache(),
	}

	svmbackups.OnChange(ctx, scheduleVMBackupControllerName, svmbackupHandler.OnChanged)
	svmbackups.OnRemove(ctx, scheduleVMBackupControllerName, svmbackupHandler.OnRemove)
	cronJobs.OnChange(ctx, cronJobControllerName, svmbackupHandler.OnCronjobChanged)
	vmBackups.OnChange(ctx, vmBackupControllerName, svmbackupHandler.OnVMBackupChange)
	vmBackups.OnRemove(ctx, vmBackupControllerName, svmbackupHandler.OnVMBackupRemove)
	lhbackups.OnChange(ctx, longhornBackupControllerName, svmbackupHandler.OnLHBackupChanged)
	return nil
}

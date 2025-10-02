package schedulevmbackup

import (
	"time"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/harvester/harvester/pkg/util"
)

func (h *svmbackupHandler) OnCronjobChanged(_ string, cronJob *batchv1.CronJob) (*batchv1.CronJob, error) {
	if cronJob == nil || cronJob.DeletionTimestamp != nil || cronJob.Status.LastScheduleTime == nil {
		return cronJob, nil
	}

	svmbackup := util.ResolveSVMBackupRef(h.svmbackupCache, cronJob)
	if svmbackup == nil {
		return nil, nil
	}

	// cronJob.Status.LastScheduleTime could be out-of-date if the scheulde experience suspend and resume
	// if this is happened, we should wait for the next reconcile
	if time.Since(cronJob.Status.LastScheduleTime.Time) > time.Minute {
		return nil, nil
	}

	timestamp := cronJob.Status.LastScheduleTime.Format(timeFormat)
	_, err := getVMBackup(h, svmbackup, timestamp)
	if err == nil {
		return cronJob, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	// Skip the cron job if the VM is running on a node which is going into / is in maintenance mode.
	node, err := h.getNodeFromBackupSource(svmbackup)
	if err != nil {
		return nil, err
	}
	if util.IsNodeInMaintenanceMode(node) {
		logrus.WithFields(logrus.Fields{
			"name":      cronJob.Name,
			"namespace": cronJob.Namespace,
			"node":      node.Name,
		}).Info("Skip cron job since the node is going into or is already in maintenance mode.")
		return nil, nil
	}

	if _, err := newVMBackups(h, svmbackup, timestamp); err != nil {
		return nil, err
	}

	return nil, nil
}

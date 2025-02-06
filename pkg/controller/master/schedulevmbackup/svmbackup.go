package schedulevmbackup

import (
	"fmt"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	utilHelm "github.com/harvester/harvester/pkg/util/helm"
)

const (
	releaseAppHarvesterName = "harvester"
)

func deleteCronJob(h *svmbackupHandler, svmbackup *harvesterv1.ScheduleVMBackup) error {
	cronJob, err := getCronJob(h, svmbackup)
	if errors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	propagation := metav1.DeletePropagationForeground
	return h.cronJobsClient.Delete(cronJob.Namespace, cronJob.Name, &metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// The cronjob actually doesn't do anything in its own job, it's utilized to trigger OnCronjobChanged()
// In OnCronjobChanged(), controller will create VMBackup for VM backup/snapshot
func createCronJob(h *svmbackupHandler, svmbackup *harvesterv1.ScheduleVMBackup) (*batchv1.CronJob, error) {
	backoffLimit := int32(cronJobBackoffLimit)
	jobImage, err := utilHelm.FetchImageFromHelmValues(h.clientset, h.namespace,
		releaseAppHarvesterName, []string{"generalJob", "image"})
	if err != nil {
		return nil, fmt.Errorf("failed to get harvester image (%s): %v", jobImage.ImageName(), err)
	}

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName(svmbackup),
			Namespace: cronJobNamespace,
			Annotations: map[string]string{
				util.AnnotationSVMBackupID: ref.Construct(svmbackup.Namespace, svmbackup.Name),
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          svmbackup.Spec.Cron,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name: cronJobName(svmbackup),
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:            cronJobName(svmbackup),
									Image:           jobImage.ImageName(),
									Command:         []string{cronJobCmd},
									Args:            []string{cronJobArg},
									Resources:       corev1.ResourceRequirements{},
									ImagePullPolicy: corev1.PullIfNotPresent,
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		},
	}
	return h.cronJobsClient.Create(cronJob)
}

func (h *svmbackupHandler) OnChanged(_ string, svmbackup *harvesterv1.ScheduleVMBackup) (*harvesterv1.ScheduleVMBackup, error) {
	if svmbackup == nil || svmbackup.DeletionTimestamp != nil {
		return svmbackup, nil
	}

	defer func() {
		if err := updateVMBackups(h, svmbackup); err != nil {
			logrus.Infof("OnChanged svmbackup %v/%v update vmbackups err %v", svmbackup.Namespace, svmbackup.Name, err)
		}
	}()

	_, err := getCronJob(h, svmbackup)
	if errors.IsNotFound(err) {
		if _, err := createCronJob(h, svmbackup); err != nil {
			return nil, err
		}

		h.svmbackupController.Enqueue(svmbackup.Namespace, svmbackup.Name)
		return svmbackup, nil
	}

	if err != nil {
		return nil, err
	}

	if err := updateResumeOrSuspend(h, svmbackup); err != nil {
		return nil, err
	}

	if err := updateCronExpression(h, svmbackup); err != nil {
		return nil, err
	}

	return svmbackup, nil
}

func (h *svmbackupHandler) OnRemove(_ string, svmbackup *harvesterv1.ScheduleVMBackup) (*harvesterv1.ScheduleVMBackup, error) {
	if svmbackup == nil {
		return nil, nil
	}

	err := deleteCronJob(h, svmbackup)
	return svmbackup, err
}

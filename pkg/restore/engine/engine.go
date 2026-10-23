package engine

import (
	"context"
	"errors"
	"fmt"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const RestoreJobCompletionFinalizer = "harvesterhci.io/restore-job-completion"

var (
	ErrRetryLater       = errors.New("retry later error")
	ErrRestoreJobFailed = errors.New("restore job failed")
)

// ReleaseRestoreJob allows the Job TTL controller to remove a completed
// restore Job. Restore Jobs retain this finalizer until their completion has
// been persisted in the owning VirtualMachineRestore, preventing a controller
// outage or informer lag from losing the only durable completion record.
func ReleaseRestoreJob(jobClient ctlbatchv1.JobClient, namespace, name string) error {
	return releaseRestoreJob(jobClient, namespace, name, false)
}

// ReleaseSucceededJob validates the retained Job before accepting
// persisted progress=100 as completion. This also protects restores whose
// progress was recorded prematurely by an older controller. A missing Job is
// safe once completion is persisted and its finalizer has already been released.
func ReleaseSucceededJob(jobClient ctlbatchv1.JobClient, namespace, name string) error {
	err := releaseRestoreJob(jobClient, namespace, name, true)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrRetryLater), errors.Is(err, ErrRestoreJobFailed):
		return err
	default:
		// An API error while confirming/releasing completion is not evidence of
		// failed data restoration. Keep the finalizer and retry confirmation.
		return fmt.Errorf("confirming restore job %s/%s: %w: %w", namespace, name, ErrRetryLater, err)
	}
}

func releaseRestoreJob(jobClient ctlbatchv1.JobClient, namespace, name string, requireSuccess bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		job, err := jobClient.Get(namespace, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if requireSuccess && job.Status.Succeeded == 0 {
			if job.Status.Failed > 0 {
				return fmt.Errorf("%w: %s/%s", ErrRestoreJobFailed, namespace, name)
			}
			return ErrRetryLater
		}

		finalizers := make([]string, 0, len(job.Finalizers))
		found := false
		for _, finalizer := range job.Finalizers {
			if finalizer == RestoreJobCompletionFinalizer {
				found = true
				continue
			}
			finalizers = append(finalizers, finalizer)
		}
		if !found {
			return nil
		}

		job = job.DeepCopy()
		job.Finalizers = append([]string(nil), finalizers...)
		_, err = jobClient.Update(job)
		return err
	})
}

// DeleteRestoreJob releases completion protection before deleting a restore
// Job during VirtualMachineRestore removal.
func DeleteRestoreJob(jobClient ctlbatchv1.JobClient, namespace, name string) error {
	if err := ReleaseRestoreJob(jobClient, namespace, name); err != nil {
		return err
	}
	err := jobClient.Delete(namespace, name, &metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// RestoreEngine defines the interface for restoring VM volumes
type RestoreEngine interface {
	// Reconcile ensures the per-volume restore at volIndex matches desired state:
	// it creates the restored PVC if missing, otherwise checks status. Idempotent.
	Reconcile(vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup, volIndex int) error

	// UpdateProgress updates the restore progress for a specific volume
	UpdateProgress(vr *harvesterv1.VolumeRestore) (int64, error)

	// Delete handles cleanup of restore resources for a specific volume
	Delete(vmr *harvesterv1.VirtualMachineRestore, volIndex int) error

	// RegisterWatchers gives the engine an opportunity to register its own
	// informer event handlers (e.g. on Jobs it creates) and call enqueue to
	// trigger reconcile on the owning VMRestore. Engines that don't need any
	// extra watchers should implement this as a no-op.
	RegisterWatchers(ctx context.Context, enqueue func(namespace, name string))
}

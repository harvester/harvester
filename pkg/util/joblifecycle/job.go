package joblifecycle

import (
	"fmt"
	"slices"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
)

const CompletionFinalizer = "harvesterhci.io/job-completion"

// Release allows a Job to be collected after its result has been persisted.
// A live lookup on every attempt preserves other finalizers and checks identity.
func Release(client ctlbatchv1.JobClient, job *batchv1.Job) error {
	if job.UID == "" {
		return fmt.Errorf("job %s/%s has no UID", job.Namespace, job.Name)
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.Get(job.Namespace, job.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.UID != job.UID {
			return fmt.Errorf("job %s/%s has been replaced", job.Namespace, job.Name)
		}
		if !slices.Contains(current.Finalizers, CompletionFinalizer) {
			return nil
		}
		updated := current.DeepCopy()
		updated.Finalizers = slices.DeleteFunc(updated.Finalizers, func(finalizer string) bool {
			return finalizer == CompletionFinalizer
		})
		_, err = client.Update(updated)
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	})
}

// Delete releases a Job and requests background deletion without waiting for GC.
func Delete(client ctlbatchv1.JobClient, job *batchv1.Job) error {
	if err := Release(client, job); err != nil {
		return err
	}
	err := client.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
		Preconditions:     &metav1.Preconditions{UID: &job.UID},
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

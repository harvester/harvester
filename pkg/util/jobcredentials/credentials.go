package jobcredentials

import (
	"fmt"
	"reflect"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func Name(jobName string) string {
	return wranglername.SafeConcatName(jobName, "credentials")
}

type Client struct {
	Jobs    ctlbatchv1.JobClient
	Secrets ctlcorev1.SecretClient
}

// Create keeps Pods from starting before their credentials have a valid Job
// owner. Ensure resumes the Job after creating or validating its Secret.
func (c Client) Create(job *batchv1.Job) (*batchv1.Job, error) {
	job = job.DeepCopy()
	job.Spec.Suspend = ptr.To(true)
	return c.Jobs.Create(job)
}

// Ensure also repairs credentials after a controller restart or Secret loss.
// Finished and deleting Jobs do not need credentials to report their results.
func (c Client) Ensure(job *batchv1.Job, data func() (map[string][]byte, error)) error {
	if !pending(job) {
		return nil
	}
	if job.UID == "" {
		return fmt.Errorf("job %s/%s has no UID", job.Namespace, job.Name)
	}
	values, err := data()
	if err != nil {
		return err
	}
	owner := metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: Name(job.Name), Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Type: corev1.SecretTypeOpaque,
		Data: values,
	}
	if err := c.ensureSecret(secret); err != nil {
		return err
	}
	if !ptr.Deref(job.Spec.Suspend, false) {
		return nil
	}
	updated := job.DeepCopy()
	updated.Spec.Suspend = ptr.To(false)
	_, err = c.Jobs.Update(updated)
	return err
}

func (c Client) ensureSecret(desired *corev1.Secret) error {
	current, err := c.Secrets.Get(desired.Namespace, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Secrets.Create(desired)
		return err
	}
	if err != nil {
		return err
	}
	if current.DeletionTimestamp != nil {
		return fmt.Errorf("job credentials %s/%s are being deleted", current.Namespace, current.Name)
	}
	if !sameOwner(current.OwnerReferences, desired.OwnerReferences) {
		return fmt.Errorf("job credentials %s/%s belong to another owner", current.Namespace, current.Name)
	}
	if reflect.DeepEqual(current.Data, desired.Data) {
		return nil
	}
	updated := current.DeepCopy()
	updated.Data = desired.Data
	_, err = c.Secrets.Update(updated)
	return err
}

func sameOwner(current, desired []metav1.OwnerReference) bool {
	if len(current) != 1 || len(desired) != 1 {
		return false
	}
	a, b := current[0], desired[0]
	return a.UID != "" && a.UID == b.UID && a.Name == b.Name && a.Kind == b.Kind && a.APIVersion == b.APIVersion
}

func pending(job *batchv1.Job) bool {
	if job.DeletionTimestamp != nil || job.Status.Succeeded > 0 {
		return false
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status == corev1.ConditionTrue &&
			(condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
			return false
		}
	}
	return true
}

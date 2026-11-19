package jobcredentials

import (
	"fmt"
	"maps"
	"strings"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
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
	job.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(false)
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = map[string]string{}
	}
	maps.Copy(job.Spec.Template.Labels, map[string]string{
		joblifecycle.LabelParentUID:  job.Labels[joblifecycle.LabelParentUID],
		joblifecycle.LabelParentKind: job.Labels[joblifecycle.LabelParentKind],
	})
	if job.Spec.Template.Annotations == nil {
		job.Spec.Template.Annotations = map[string]string{}
	}
	maps.Copy(job.Spec.Template.Annotations, map[string]string{
		joblifecycle.AnnotationParentName:      job.Annotations[joblifecycle.AnnotationParentName],
		joblifecycle.AnnotationParentNamespace: job.Annotations[joblifecycle.AnnotationParentNamespace],
	})
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
	if err := c.ensureSecret(job, data); err != nil {
		return err
	}
	if !ptr.Deref(job.Spec.Suspend, false) {
		return nil
	}
	return c.resumeJob(job)
}

// resumeJob reloads on each attempt because the Job controller can update status
// between creation and resuming. Never resume a replacement or completed Job.
func (c Client) resumeJob(job *batchv1.Job) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := c.Jobs.Get(job.Namespace, job.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.UID != job.UID {
			return fmt.Errorf("job %s/%s has been replaced", job.Namespace, job.Name)
		}
		if !pending(current) || !ptr.Deref(current.Spec.Suspend, false) {
			return nil
		}
		updated := current.DeepCopy()
		updated.Spec.Suspend = ptr.To(false)
		_, err = c.Jobs.Update(updated)
		return err
	})
}

func (c Client) ensureSecret(job *batchv1.Job, data func() (map[string][]byte, error)) error {
	current, err := c.Secrets.Get(job.Namespace, Name(job.Name), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.createSecret(job, data)
	}
	if err != nil {
		return err
	}
	return validateSecret(job, current)
}

func (c Client) createSecret(job *batchv1.Job, data func() (map[string][]byte, error)) error {
	values, err := data()
	if err != nil {
		return err
	}
	owner := metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: Name(job.Name), Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Type: corev1.SecretTypeOpaque, Immutable: ptr.To(true), Data: values,
	}
	_, err = c.Secrets.Create(desired)
	return err
}

func validateSecret(job *batchv1.Job, current *corev1.Secret) error {
	owner := metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))
	if current.DeletionTimestamp != nil {
		return fmt.Errorf("job credentials are being deleted")
	}
	if !sameOwner(current.OwnerReferences, []metav1.OwnerReference{*owner}) {
		return fmt.Errorf("job credentials belong to another owner")
	}
	for _, container := range job.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				continue
			}
			ref := env.ValueFrom.SecretKeyRef
			if ref.Name != current.Name {
				continue
			}
			value := current.Data[ref.Key]
			if len(value) == 0 || strings.ContainsRune(string(value), 0) {
				return fmt.Errorf("job credentials have an invalid %s", ref.Key)
			}
		}
	}
	return nil
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

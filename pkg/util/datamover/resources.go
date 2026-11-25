package datamover

import (
	"context"
	"errors"
	"fmt"
	"slices"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

var ErrPending = errors.New("waiting for repository workload resources")

const (
	BackupKind  = "VirtualMachineBackup"
	RestoreKind = "VirtualMachineRestore"
)

// StopJob waits for consumers to disappear before storage can be released.
func StopJob(client kubernetes.Interface, parent metav1.Object, kind, name string) error {
	if err := deleteJob(client, parent, kind, name); err != nil {
		return err
	}
	return waitJob(client, name)
}

func waitJob(client kubernetes.Interface, name string) error {
	if err := waitJobPods(client, name); err != nil {
		return err
	}
	_, err := client.BatchV1().Jobs(joblifecycle.Namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrPending
}

func waitJobPods(client kubernetes.Interface, name string) error {
	pods, err := client.CoreV1().Pods(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set{batchv1.JobNameLabel: name}.AsSelector().String(),
	})
	if err != nil {
		return err
	}
	if len(pods.Items) != 0 {
		return ErrPending
	}
	return nil
}

func deleteJob(client kubernetes.Interface, parent metav1.Object, kind, name string) error {
	job, err := trackedJob(client, parent, kind, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := releaseJob(client, job); err != nil {
		return err
	}
	err = client.BatchV1().Jobs(job.Namespace).Delete(context.Background(), name, metav1.DeleteOptions{
		Preconditions:     &metav1.Preconditions{UID: &job.UID},
		PropagationPolicy: ptr.To(metav1.DeletePropagationForeground),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func trackedJob(
	client kubernetes.Interface, parent metav1.Object, kind, name string,
) (*batchv1.Job, error) {
	job, err := client.BatchV1().Jobs(joblifecycle.Namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}
	if err := joblifecycle.ValidateJob(parent, kind, job); err != nil {
		return nil, err
	}
	return job, nil
}

func releaseJob(client kubernetes.Interface, job *batchv1.Job) error {
	if !slices.Contains(job.Finalizers, joblifecycle.CompletionFinalizer) {
		return nil
	}
	job.Finalizers = slices.DeleteFunc(job.Finalizers, func(finalizer string) bool {
		return finalizer == joblifecycle.CompletionFinalizer
	})
	_, err := client.BatchV1().Jobs(job.Namespace).Update(context.Background(), job, metav1.UpdateOptions{})
	return err
}

func Detached(client kubernetes.Interface, pvName string) error {
	attachments, err := client.StorageV1().VolumeAttachments().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, attachment := range attachments.Items {
		if ptr.Deref(attachment.Spec.Source.PersistentVolumeName, "") == pvName {
			return ErrPending
		}
	}
	return nil
}

// Unused also waits for consumers outside the transfer Job. Never delete those
// Pods or bypass PVC protection to force a handoff.
func Unused(client kubernetes.Interface, namespace, claim string) error {
	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claim {
				return ErrPending
			}
		}
	}
	return nil
}

// SnapshotSource accepts the mirrored dataSourceRef populated by Kubernetes,
// but rejects a different source or a cross-namespace reference.
func SnapshotSource(pvc *corev1.PersistentVolumeClaim, name string) error {
	source, ref := pvc.Spec.DataSource, pvc.Spec.DataSourceRef
	if source == nil || source.Kind != "VolumeSnapshot" || source.Name != name ||
		ptr.Deref(source.APIGroup, "") != "snapshot.storage.k8s.io" {
		return fmt.Errorf("backup clone PVC has a different source")
	}
	if ref != nil && (ref.Kind != source.Kind || ref.Name != source.Name ||
		ptr.Deref(ref.APIGroup, "") != ptr.Deref(source.APIGroup, "") || ptr.Deref(ref.Namespace, "") != "") {
		return fmt.Errorf("backup clone PVC has a different source reference")
	}
	return nil
}

func DeleteClaim(client kubernetes.Interface, parent metav1.Object, kind, name string) error {
	pvc, err := trackedClaim(client, parent, kind, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return deleteClaim(client, pvc)
}

func trackedClaim(
	client kubernetes.Interface, parent metav1.Object, kind, name string,
) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}
	if err := joblifecycle.Validate(parent, kind, pvc.ObjectMeta); err != nil {
		return nil, err
	}
	return pvc, nil
}

func deleteClaim(client kubernetes.Interface, pvc *corev1.PersistentVolumeClaim) error {
	err := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(
		context.Background(), pvc.Name, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &pvc.UID},
		},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return ErrPending
}

// ValidateClaim prevents a same-name resource from being adopted by a transfer.
func ValidateClaim(parent metav1.Object, kind string, current, desired *corev1.PersistentVolumeClaim) error {
	if err := joblifecycle.Validate(parent, kind, current.ObjectMeta); err != nil {
		return err
	}
	if current.DeletionTimestamp != nil {
		return ErrPending
	}
	currentMode := ptr.Deref(current.Spec.VolumeMode, corev1.PersistentVolumeFilesystem)
	desiredMode := ptr.Deref(desired.Spec.VolumeMode, corev1.PersistentVolumeFilesystem)
	if ptr.Deref(current.Spec.StorageClassName, "") != ptr.Deref(desired.Spec.StorageClassName, "") ||
		currentMode != desiredMode ||
		!slices.Equal(current.Spec.AccessModes, desired.Spec.AccessModes) {
		return fmt.Errorf("PVC %s/%s has different storage properties", current.Namespace, current.Name)
	}
	if current.Spec.Resources.Requests.Storage().Cmp(*desired.Spec.Resources.Requests.Storage()) < 0 {
		return fmt.Errorf("PVC %s/%s is smaller than the requested volume", current.Namespace, current.Name)
	}
	return nil
}

func StopJobs(client kubernetes.Interface, parent metav1.Object, kind string) error {
	jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, kind),
	})
	if err != nil {
		return err
	}
	for _, job := range jobs.Items {
		if joblifecycle.ValidateJob(parent, kind, &job) != nil {
			continue
		}
		if err := StopJob(client, parent, kind, job.Name); err != nil {
			return err
		}
	}
	return stopPods(client, parent, kind)
}

func stopPods(client kubernetes.Interface, parent metav1.Object, kind string) error {
	pods, err := client.CoreV1().Pods(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, kind),
	})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if err := deletePod(client, parent, kind, &pod); err != nil {
			return err
		}
	}
	if len(pods.Items) != 0 {
		return ErrPending
	}
	return nil
}

func deletePod(client kubernetes.Interface, parent metav1.Object, kind string, pod *corev1.Pod) error {
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.Kind != "Job" || owner.UID == "" {
		return fmt.Errorf("tracked transfer Pod has an unexpected owner")
	}
	meta := pod.ObjectMeta.DeepCopy()
	meta.OwnerReferences = nil
	if err := joblifecycle.Validate(parent, kind, *meta); err != nil {
		return err
	}
	err := client.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &pod.UID},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

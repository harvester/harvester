package k8s

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/transport"
)

const (
	UserAddonResourceName         = "rke-user-addon"
	UserAddonsIncludeResourceName = "rke-user-includes-addons"
)

type JobStatus struct {
	Completed bool
	Created   bool
	Removing  bool
}

func ApplyK8sSystemJob(jobYaml, kubeConfigPath string, k8sWrapTransport transport.WrapperFunc, timeout int, resourceName string, addonUpdated bool) error {
	job := v1.Job{}
	if err := DecodeYamlResource(&job, jobYaml); err != nil {
		return err
	}
	if job.Namespace == metav1.NamespaceNone {
		job.Namespace = metav1.NamespaceSystem
	}
	k8sClient, err := NewClient(kubeConfigPath, k8sWrapTransport)
	if err != nil {
		return err
	}

	var jobStatus JobStatus

	// If the job is still removing, attempt to wait until it has been deleted
	// If the job is "stuck", apply will never succeed and requires outside intervention
	if err := retryToWithTimeout(func(clientset *kubernetes.Clientset, i interface{}) error {
		if jobStatus, err = GetK8sJobStatus(k8sClient, job.Name, job.Namespace); err != nil {
			return err
		}
		if !jobStatus.Removing {
			return nil
		}
		logrus.Debugf("[k8s] waiting for job %s to delete..", job.Name)
		return fmt.Errorf("[k8s] Job [%s] deletion timed out. Consider increasing addon_job_timeout value", job.Name)
	}, k8sClient, job, timeout); err != nil {
		return err
	}

	// remove the existing job if addon config has changed or the job isn't completed
	// always remove the existing job for user addons to rerun kubectl apply for user addons
	if addonUpdated || (jobStatus.Created && !jobStatus.Completed) ||
		resourceName == UserAddonResourceName || resourceName == UserAddonsIncludeResourceName {
		logrus.Debugf("[k8s] replacing job %s for %s", job.Name, resourceName)
		if err := DeleteK8sSystemJob(jobYaml, k8sClient, timeout); err != nil {
			return err
		}
	}
	if _, err = k8sClient.BatchV1().Jobs(job.Namespace).Create(context.TODO(), &job, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.Debugf("[k8s] Job %s already exists..", job.Name)
			return nil
		}
		return err
	}
	logrus.Debugf("[k8s] waiting for job %s to complete..", job.Name)
	return retryToWithTimeout(ensureJobCompleted, k8sClient, job, timeout)
}

func DeleteK8sSystemJob(jobYaml string, k8sClient *kubernetes.Clientset, timeout int) error {
	job := v1.Job{}
	if err := DecodeYamlResource(&job, jobYaml); err != nil {
		return err
	}
	if err := deleteK8sJob(k8sClient, job.Name, job.Namespace); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else { // ignoring NotFound errors
		//Jobs take longer to delete than to complete, 2 x the timeout
		if err := retryToWithTimeout(ensureJobDeleted, k8sClient, job, timeout*2); err != nil {
			return err
		}
	}
	return nil
}

func DeleteK8sJobIfExists(k8sClient *kubernetes.Clientset, name, namespace string) error {
	if err := deleteK8sJob(k8sClient, name, namespace); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func ensureJobCompleted(k8sClient *kubernetes.Clientset, j interface{}) error {
	job := j.(v1.Job)

	jobStatus, err := GetK8sJobStatus(k8sClient, job.Name, job.Namespace)
	if err != nil {
		return fmt.Errorf("Failed to get job complete status for job %s in namespace %s: %v", job.Name, job.Namespace, err)
	}
	if jobStatus.Completed {
		logrus.Debugf("[k8s] Job %s in namespace %s completed successfully", job.Name, job.Namespace)
		return nil
	}
	return fmt.Errorf("Failed to get job complete status for job %s in namespace %s", job.Name, job.Namespace)
}

func ensureJobDeleted(k8sClient *kubernetes.Clientset, j interface{}) error {
	job := j.(v1.Job)
	_, err := k8sClient.BatchV1().Jobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// this is the "true" return of the function
			return nil
		}
		return err
	}
	return fmt.Errorf("[k8s] Job [%s] deletion timed out. Consider increasing addon_job_timeout value", job.Name)
}

func deleteK8sJob(k8sClient *kubernetes.Clientset, name, namespace string) error {
	deletePolicy := metav1.DeletePropagationForeground
	return k8sClient.BatchV1().Jobs(namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		})
}

func getK8sJob(k8sClient *kubernetes.Clientset, name, namespace string) (*v1.Job, error) {
	return k8sClient.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func GetK8sJobStatus(k8sClient *kubernetes.Clientset, name, namespace string) (JobStatus, error) {
	existingJob, err := getK8sJob(k8sClient, name, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return JobStatus{}, nil
		}
		return JobStatus{}, err
	}
	for _, condition := range existingJob.Status.Conditions {
		if condition.Type == v1.JobComplete && condition.Status == corev1.ConditionTrue {
			return JobStatus{
				Created:   true,
				Completed: true,
				Removing:  existingJob.DeletionTimestamp != nil,
			}, err
		}
	}
	return JobStatus{
		Created:   true,
		Completed: false,
		Removing:  existingJob.DeletionTimestamp != nil,
	}, nil
}

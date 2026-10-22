package snapshotcheck

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const MissingExitCode = 42

type Result string

const (
	Pending Result = "pending"
	Found   Result = "found"
	Missing Result = "missing"
	Failed  Result = "failed"
)

type CheckJobOptions struct {
	EngineName      string
	ContainerName   string
	MissingExitCode int32
	JobName         string
	GetJob          func(name string) (*batchv1.Job, error)
	CreateJob       func(name string) error
}

func CheckJob(ctx context.Context, clientset kubernetes.Interface, opts CheckJobOptions) (Result, error) {
	job, err := opts.GetJob(opts.JobName)
	if err == nil {
		return JobResult(ctx, clientset, job, opts.EngineName, opts.ContainerName, opts.MissingExitCode)
	}
	if !apierrors.IsNotFound(err) {
		return Pending, err
	}
	if err := opts.CreateJob(opts.JobName); err != nil {
		return Pending, err
	}
	return Pending, nil
}

func JobResult(ctx context.Context,
	clientset kubernetes.Interface,
	job *batchv1.Job,
	engineName, containerName string,
	missingExitCode int32,
) (Result, error) {
	if job.Status.Failed > 0 {
		return failedJobResult(ctx, clientset, job, engineName, containerName, missingExitCode)
	}
	if job.Status.Succeeded == 0 {
		return Pending, nil
	}
	return Found, nil
}

func failedJobResult(
	ctx context.Context,
	clientset kubernetes.Interface,
	job *batchv1.Job,
	engineName, containerName string,
	missingExitCode int32,
) (Result, error) {
	exitCode, foundExitCode, err := JobExitCode(ctx, clientset, job, engineName, containerName)
	if err != nil {
		return Pending, err
	}
	if foundExitCode && exitCode == missingExitCode {
		return Missing, nil
	}
	return Failed, nil
}

func JobExitCode(
	ctx context.Context,
	clientset kubernetes.Interface,
	job *batchv1.Job,
	engineName, containerName string,
) (int32, bool, error) {
	pods, err := clientset.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
	})
	if err != nil {
		return 0, false, fmt.Errorf("listing pods for %s snapshot check job %s/%s: %w", engineName, job.Namespace, job.Name, err)
	}
	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name != containerName || status.State.Terminated == nil {
				continue
			}
			return status.State.Terminated.ExitCode, true, nil
		}
	}
	return 0, false, nil
}

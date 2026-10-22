package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestReleaseRestoreJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "restore",
			Namespace:  "default",
			Finalizers: []string{"example.io/other", RestoreJobCompletionFinalizer},
		},
	}
	clientset := fake.NewSimpleClientset(job)
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.NoError(t, ReleaseRestoreJob(jobClient, job.Namespace, job.Name))

	updated, err := clientset.BatchV1().Jobs(job.Namespace).Get(
		context.Background(),
		job.Name,
		metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"example.io/other"}, updated.Finalizers)
}

func TestDeleteRestoreJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "restore",
			Namespace:  "default",
			Finalizers: []string{RestoreJobCompletionFinalizer},
		},
	}
	clientset := fake.NewSimpleClientset(job)
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.NoError(t, DeleteRestoreJob(jobClient, job.Namespace, job.Name))
	_, err := clientset.BatchV1().Jobs(job.Namespace).Get(
		context.Background(),
		job.Name,
		metav1.GetOptions{},
	)
	require.Error(t, err)
}

func TestReleaseSucceededJob(t *testing.T) {
	tests := []struct {
		name      string
		status    batchv1.JobStatus
		wantError bool
		wantRetry bool
	}{
		{name: "pending", wantError: true, wantRetry: true},
		{name: "running", status: batchv1.JobStatus{Active: 1}, wantError: true, wantRetry: true},
		{name: "failed after streaming all bytes", status: batchv1.JobStatus{Failed: 1}, wantError: true},
		{name: "succeeded", status: batchv1.JobStatus{Succeeded: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "restore", Namespace: "default",
					Finalizers: []string{RestoreJobCompletionFinalizer},
				},
				Status: tt.status,
			}
			clientset := fake.NewSimpleClientset(job)
			err := ReleaseSucceededJob(fakeclients.JobClient(clientset.BatchV1().Jobs), job.Namespace, job.Name)
			if tt.wantError {
				require.Error(t, err)
				if tt.wantRetry {
					require.ErrorIs(t, err, ErrRetryLater)
				} else {
					require.NotErrorIs(t, err, ErrRetryLater)
				}
			} else {
				require.NoError(t, err)
			}
			updated, err := clientset.BatchV1().Jobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
			require.NoError(t, err)
			if tt.wantError {
				require.Equal(t, job.Finalizers, updated.Finalizers)
			} else {
				require.Empty(t, updated.Finalizers)
			}
		})
	}
}

func TestReleaseSucceededJobAlreadyRemoved(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)
	require.NoError(t, ReleaseSucceededJob(jobClient, "default", "restore"))
	_, err := jobClient.Get("default", "restore", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestReleaseSucceededJobRetriesAPIErrors(t *testing.T) {
	for _, verb := range []string{"get", "update"} {
		t.Run(verb, func(t *testing.T) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "restore", Namespace: "default",
					Finalizers: []string{RestoreJobCompletionFinalizer},
				},
				Status: batchv1.JobStatus{Succeeded: 1},
			}
			clientset := fake.NewSimpleClientset(job)
			apiErr := errors.New("API unavailable")
			clientset.PrependReactor(verb, "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			err := ReleaseSucceededJob(fakeclients.JobClient(clientset.BatchV1().Jobs), job.Namespace, job.Name)
			require.ErrorIs(t, err, ErrRetryLater)
			require.ErrorIs(t, err, apiErr)
			require.NotErrorIs(t, err, ErrRestoreJobFailed)
		})
	}
}

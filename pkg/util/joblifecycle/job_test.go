package joblifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clienttesting "k8s.io/client-go/testing"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestRelease(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "restore",
			UID:        "job-uid",
			Namespace:  "default",
			Finalizers: []string{"example.io/other", joblifecycle.CompletionFinalizer},
		},
	}
	clientset := fake.NewSimpleClientset(job)
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.NoError(t, joblifecycle.Release(jobClient, job))

	updated, err := clientset.BatchV1().Jobs(job.Namespace).Get(
		context.Background(),
		job.Name,
		metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"example.io/other"}, updated.Finalizers)
}

func TestDelete(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "restore",
			UID:        "job-uid",
			Namespace:  "default",
			Finalizers: []string{joblifecycle.CompletionFinalizer},
		},
	}
	clientset := fake.NewSimpleClientset(job)
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.NoError(t, joblifecycle.Delete(jobClient, job))
	_, err := clientset.BatchV1().Jobs(job.Namespace).Get(
		context.Background(),
		job.Name,
		metav1.GetOptions{},
	)
	require.Error(t, err)
}

func TestReleaseAlreadyRemoved(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	jobClient := fakeclients.JobClient(clientset.BatchV1().Jobs)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "job-uid"}}
	require.NoError(t, joblifecycle.Release(jobClient, job))
	require.NoError(t, joblifecycle.Delete(jobClient, job))
	_, err := jobClient.Get("default", "restore", metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestReleaseReturnsAPIErrors(t *testing.T) {
	for _, verb := range []string{"get", "update"} {
		t.Run(verb, func(t *testing.T) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "restore", Namespace: "default", UID: "job-uid",
					Finalizers: []string{joblifecycle.CompletionFinalizer},
				},
				Status: batchv1.JobStatus{Succeeded: 1},
			}
			clientset := fake.NewSimpleClientset(job)
			apiErr := errors.New("API unavailable")
			clientset.PrependReactor(verb, "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			err := joblifecycle.Release(fakeclients.JobClient(clientset.BatchV1().Jobs), job)
			require.ErrorIs(t, err, apiErr)
		})
	}
}

func TestReleaseChecksUID(t *testing.T) {
	for _, uid := range []string{"", "old-uid"} {
		t.Run(uid, func(t *testing.T) {
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
				Name: "job", Namespace: "default", UID: "current-uid",
				Finalizers: []string{joblifecycle.CompletionFinalizer},
			}}
			client := fake.NewSimpleClientset(job)
			expected := job.DeepCopy()
			expected.SetUID(types.UID(uid))
			require.Error(t, joblifecycle.Release(fakeclients.JobClient(client.BatchV1().Jobs), expected))
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestReleaseRetriesConflict(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", Namespace: "default", UID: "job-uid",
		Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	client := fake.NewSimpleClientset(job)
	updates := 0
	client.PrependReactor("update", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		updates++
		if updates > 1 {
			return false, nil, nil
		}
		changed := job.DeepCopy()
		changed.Finalizers = append(changed.Finalizers, "example.io/concurrent")
		err := client.Tracker().Update(batchv1.SchemeGroupVersion.WithResource("jobs"), changed, job.Namespace)
		require.NoError(t, err)
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, job.Name, errors.New("conflict"))
	})
	require.NoError(t, joblifecycle.Release(fakeclients.JobClient(client.BatchV1().Jobs), job))
	updated, err := client.BatchV1().Jobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, updates)
	require.Equal(t, []string{"example.io/concurrent"}, updated.Finalizers)
	require.Equal(t, []string{joblifecycle.CompletionFinalizer}, job.Finalizers, "leave the cached object unchanged")
}

func TestDeleteDoesNotWaitForGC(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", Namespace: "default", UID: "job-uid",
		Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	client := fake.NewSimpleClientset(job)
	client.PrependReactor("delete", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		opts := action.(clienttesting.DeleteAction).GetDeleteOptions()
		require.Equal(t, job.UID, *opts.Preconditions.UID)
		require.Equal(t, metav1.DeletePropagationBackground, *opts.PropagationPolicy)
		return true, nil, nil
	})
	require.NoError(t, joblifecycle.Delete(fakeclients.JobClient(client.BatchV1().Jobs), job))
	remaining, err := client.BatchV1().Jobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, remaining.Finalizers)
}

func TestDeleteReturnsAPIError(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", Namespace: "default", UID: "job-uid",
		Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	client := fake.NewSimpleClientset(job)
	deleteErr := errors.New("delete unavailable")
	client.PrependReactor("delete", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, deleteErr
	})
	require.ErrorIs(t, joblifecycle.Delete(fakeclients.JobClient(client.BatchV1().Jobs), job), deleteErr)
}

func TestReleaseAfterConcurrentRemoval(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "job", Namespace: "default", UID: "job-uid",
		Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	client := fake.NewSimpleClientset(job)
	client.PrependReactor("update", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "jobs"}, job.Name)
	})
	require.NoError(t, joblifecycle.Release(fakeclients.JobClient(client.BatchV1().Jobs), job))
}

package jobcredentials

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func testJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "job-uid"},
		Spec: batchv1.JobSpec{
			Suspend: ptr.To(true), BackoffLimit: ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Env: []corev1.EnvVar{{
					Name: "PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: Name("backup")}, Key: "password",
					}},
				}}}},
			}},
		},
	}
}

func testData() (map[string][]byte, error) {
	return map[string][]byte{"password": []byte("expected")}, nil
}

func clientFor(client *fake.Clientset) Client {
	return Client{
		Jobs: fakeclients.JobClient(client.BatchV1().Jobs), Secrets: fakeclients.SecretClient(client.CoreV1().Secrets),
	}
}

func TestCreateAndRecover(t *testing.T) {
	ctx := context.Background()
	job := testJob()
	job.Spec.Suspend = nil
	client := fake.NewSimpleClientset()
	credentials := clientFor(client)
	created, err := credentials.Create(job)
	require.NoError(t, err)
	require.True(t, *created.Spec.Suspend)
	require.Nil(t, job.Spec.Suspend, "do not mutate the caller's template")
	secrets := client.CoreV1().Secrets(job.Namespace)
	apiErr := errors.New("API unavailable")
	fail := true
	client.PrependReactor("create", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, apiErr
		}
		return false, nil, nil
	})
	require.ErrorIs(t, credentials.Ensure(created, testData), apiErr)
	stillSuspended, err := credentials.Jobs.Get(job.Namespace, job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, *stillSuspended.Spec.Suspend)
	fail = false
	require.NoError(t, credentials.Ensure(stillSuspended, testData))
	active, err := credentials.Jobs.Get(job.Namespace, job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.False(t, *active.Spec.Suspend)
	secret, err := secrets.Get(ctx, Name(job.Name), metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, metav1.IsControlledBy(secret, job))
	require.Equal(t, "Job", secret.OwnerReferences[0].Kind)
	require.NoError(t, secrets.Delete(ctx, secret.Name, metav1.DeleteOptions{}))
	require.NoError(t, credentials.Ensure(active, testData))
	repaired, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, secret.Data, repaired.Data)
	require.Equal(t, secret.OwnerReferences, repaired.OwnerReferences)
}

func TestRejectStaleCredentials(t *testing.T) {
	for _, scenario := range []string{"previous-job", "terminating", "unowned"} {
		t.Run(scenario, func(t *testing.T) {
			job := testJob()
			owner := metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: Name(job.Name), Namespace: job.Namespace, OwnerReferences: []metav1.OwnerReference{*owner},
			}}
			switch scenario {
			case "previous-job":
				secret.OwnerReferences[0].UID = "old-job-uid"
			case "terminating":
				secret.DeletionTimestamp = ptr.To(metav1.Now())
			case "unowned":
				secret.OwnerReferences = nil
			}
			client := fake.NewSimpleClientset(job, secret)
			require.Error(t, clientFor(client).Ensure(job, testData))
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb(), "leave credentials and the suspended Job untouched")
			}
		})
	}
}

func TestFinishedJobsNeedNoCredentials(t *testing.T) {
	for _, scenario := range []string{"succeeded", "failed", "deleting", "retrying"} {
		t.Run(scenario, func(t *testing.T) {
			job := testJob()
			switch scenario {
			case "succeeded":
				job.Status.Succeeded = 1
			case "failed":
				job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
			case "deleting":
				job.DeletionTimestamp = ptr.To(metav1.Now())
			case "retrying":
				job.Status.Failed = 1
			}
			calls := 0
			err := clientFor(fake.NewSimpleClientset(job)).Ensure(job, func() (map[string][]byte, error) {
				calls++
				return testData()
			})
			require.NoError(t, err)
			if scenario == "retrying" {
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls)
			}
		})
	}
}

package jobcredentials

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
)

func TestResumeRetriesConflict(t *testing.T) {
	job := testJob()
	job.ResourceVersion = "1"
	current := job.DeepCopy()
	current.ResourceVersion = "2"
	current.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue}}
	client := fake.NewSimpleClientset(current)
	attempts := 0
	client.PrependReactor("update", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		attempts++
		updated := action.(clienttesting.UpdateAction).GetObject().(*batchv1.Job)
		require.Equal(t, current.ResourceVersion, updated.ResourceVersion, "reload before every update")
		require.Equal(t, current.Status, updated.Status)
		require.Equal(t, current.Annotations, updated.Annotations)
		require.False(t, *updated.Spec.Suspend)
		if attempts > 1 {
			return false, nil, nil
		}
		current.ResourceVersion = "3"
		current.Annotations = map[string]string{"other-controller": "keep"}
		require.NoError(t, client.Tracker().Update(batchv1.SchemeGroupVersion.WithResource("jobs"), current, job.Namespace))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, job.Name, errors.New("updated"))
	})

	require.NoError(t, clientFor(client).Ensure(job, testData))
	require.Equal(t, 2, attempts)
	require.Equal(t, "1", job.ResourceVersion)
	require.True(t, *job.Spec.Suspend, "do not mutate the caller's Job")
}

func TestResumeChecksCurrentJob(t *testing.T) {
	for _, state := range []string{"completed", "failed", "deleting", "resumed", "replaced"} {
		t.Run(state, func(t *testing.T) {
			job := testJob()
			current := job.DeepCopy()
			switch state {
			case "completed":
				current.Status.Succeeded = 1
			case "failed":
				current.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
			case "deleting":
				current.DeletionTimestamp = ptr.To(metav1.Now())
			case "resumed":
				current.Spec.Suspend = ptr.To(false)
			case "replaced":
				current.UID = "another-job"
			}
			client := fake.NewSimpleClientset(current)
			err := clientFor(client).Ensure(job, testData)
			if state == "replaced" {
				require.ErrorContains(t, err, "has been replaced")
			} else {
				require.NoError(t, err)
			}
			for _, action := range client.Actions() {
				if action.GetResource().Resource == "jobs" {
					require.Equal(t, "get", action.GetVerb(), "do not update a Job that should no longer resume")
				}
			}
		})
	}
}

func TestResumeReturnsFailures(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict=%t", conflict), func(t *testing.T) {
			job := testJob()
			client := fake.NewSimpleClientset(job)
			failure := errors.New("API unavailable")
			if conflict {
				failure = apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, job.Name, failure)
			}
			attempts := 0
			client.PrependReactor("update", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				attempts++
				return true, nil, failure
			})
			err := clientFor(client).Ensure(job, testData)
			require.ErrorIs(t, err, failure)
			want := 1
			if conflict {
				want = retry.DefaultRetry.Steps
			}
			require.Equal(t, want, attempts)
		})
	}
}

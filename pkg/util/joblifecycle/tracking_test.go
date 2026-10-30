package joblifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func testParent() *harvesterv1.VirtualMachineBackup {
	return &harvesterv1.VirtualMachineBackup{ObjectMeta: metav1.ObjectMeta{
		Name: "backup", Namespace: "default", UID: "backup-uid",
	}}
}

func trackedJob(parent *harvesterv1.VirtualMachineBackup, name string) *batchv1.Job {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: parent.Namespace, UID: "job-uid",
		Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	joblifecycle.Track(&job.ObjectMeta, parent, "VirtualMachineBackup")
	return job
}

func TestValidateJob(t *testing.T) {
	for _, scenario := range []string{
		"valid", "terminating", "foreign", "untracked", "owned", "wrong-name", "wrong-namespace", "missing-uid", "wrong-kind",
	} {
		t.Run(scenario, func(t *testing.T) {
			parent := testParent()
			job := trackedJob(parent, "forget")
			switch scenario {
			case "terminating":
				job.DeletionTimestamp = ptr.To(metav1.Now())
			case "foreign":
				job.Labels[joblifecycle.LabelParentUID] = "another-backup"
			case "untracked":
				job.Labels = nil
			case "owned":
				job.OwnerReferences = []metav1.OwnerReference{{UID: parent.UID}}
			case "wrong-name":
				job.Annotations[joblifecycle.AnnotationParentName] = "another-backup"
			case "wrong-namespace":
				job.Namespace = "another-namespace"
			case "wrong-kind":
				job.Labels[joblifecycle.LabelParentKind] = "VirtualMachineRestore"
			case "missing-uid":
				parent.UID = ""
			}
			err := joblifecycle.ValidateJob(parent, "VirtualMachineBackup", job)
			if scenario != "valid" && scenario != "terminating" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestParentName(t *testing.T) {
	parent := testParent()
	job := trackedJob(parent, "forget")
	require.Equal(t, parent.Name, joblifecycle.ParentName(job.ObjectMeta, "VirtualMachineBackup"))
	require.Empty(t, joblifecycle.ParentName(job.ObjectMeta, "VirtualMachineRestore"))
	delete(job.Labels, joblifecycle.LabelParentUID)
	require.Empty(t, joblifecycle.ParentName(job.ObjectMeta, "VirtualMachineBackup"))
}

func TestDeleteParentJobs(t *testing.T) {
	parent := testParent()
	objects := make([]runtime.Object, 0, 6)
	for _, scenario := range []string{"matching", "old-uid", "other-kind", "other-name", "other-namespace", "owned"} {
		job := trackedJob(parent, scenario)
		switch scenario {
		case "old-uid":
			job.Labels[joblifecycle.LabelParentUID] = "old-uid"
		case "other-kind":
			job.Labels[joblifecycle.LabelParentKind] = "VirtualMachineRestore"
		case "other-name":
			job.Annotations[joblifecycle.AnnotationParentName] = "other"
		case "other-namespace":
			job.Namespace = "other"
		case "owned":
			job.OwnerReferences = []metav1.OwnerReference{{UID: parent.UID}}
		}
		objects = append(objects, job)
	}
	client := fake.NewSimpleClientset(objects...)
	require.NoError(t, joblifecycle.DeleteJobs(fakeclients.JobClient(client.BatchV1().Jobs), parent, "VirtualMachineBackup"))
	require.NoError(t, joblifecycle.DeleteJobs(fakeclients.JobClient(client.BatchV1().Jobs), parent, "VirtualMachineBackup"))
	remaining, err := client.BatchV1().Jobs("").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, remaining.Items, 5)
	for _, job := range remaining.Items {
		require.NotEqual(t, "matching", job.Name)
		require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
	}
}

func TestDeleteParentRetries(t *testing.T) {
	for _, verb := range []string{"list", "update", "delete"} {
		t.Run(verb, func(t *testing.T) {
			parent := testParent()
			client := fake.NewSimpleClientset(trackedJob(parent, "job"))
			apiErr := errors.New("API unavailable")
			fail := true
			client.PrependReactor(verb, "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				if fail {
					return true, nil, apiErr
				}
				return false, nil, nil
			})
			jobs := fakeclients.JobClient(client.BatchV1().Jobs)
			require.ErrorIs(t, joblifecycle.DeleteJobs(jobs, parent, "VirtualMachineBackup"), apiErr)
			fail = false
			require.NoError(t, joblifecycle.DeleteJobs(jobs, parent, "VirtualMachineBackup"))
			remaining, err := jobs.List(parent.Namespace, metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, remaining.Items)
		})
	}
}

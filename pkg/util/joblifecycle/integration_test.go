package joblifecycle_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/jobcredentials"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

// Run in a disposable cluster with Harvester CRDs, without Harvester controllers/webhooks:
// HARVESTER_CLEANUP_TEST_KUBECONFIG=... go test ./pkg/util/joblifecycle -run TestCleanupLifecycle -v
func TestCleanupLifecycle(t *testing.T) {
	path := os.Getenv("HARVESTER_CLEANUP_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("set HARVESTER_CLEANUP_TEST_KUBECONFIG to a disposable cluster")
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	require.NoError(t, err)
	client, err := versioned.NewForConfig(config)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: joblifecycle.Namespace}}, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
	for _, policy := range []metav1.DeletionPropagation{metav1.DeletePropagationForeground, metav1.DeletePropagationBackground} {
		t.Run(string(policy), func(t *testing.T) {
			namespace, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "repository-lifecycle-"}}, metav1.CreateOptions{})
			require.NoError(t, err)
			parent, err := client.HarvesterhciV1beta1().VirtualMachineBackups(namespace.Name).Create(ctx, &harvesterv1.VirtualMachineBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: namespace.Name, Finalizers: []string{"test.harvesterhci.io/cleanup"}},
				Spec:       harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic, Source: corev1.TypedLocalObjectReference{APIGroup: ptr.To("kubevirt.io"), Kind: "VirtualMachine", Name: "vm"}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			jobs := fakeclients.JobClient(client.BatchV1().Jobs)
			credentials := jobcredentials.Client{Jobs: jobs, Secrets: fakeclients.SecretClient(client.CoreV1().Secrets)}
			name := joblifecycle.Name("lifecycle", parent, "volume")
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
				defer stop()
				_ = joblifecycle.DeleteJob(jobs, parent, "VirtualMachineBackup", name)
				current, getErr := client.HarvesterhciV1beta1().VirtualMachineBackups(namespace.Name).Get(cleanup, parent.Name, metav1.GetOptions{})
				if getErr == nil {
					current.Finalizers = nil
					_, _ = client.HarvesterhciV1beta1().VirtualMachineBackups(namespace.Name).Update(cleanup, current, metav1.UpdateOptions{})
				}
				_ = client.CoreV1().Namespaces().Delete(cleanup, namespace.Name, metav1.DeleteOptions{})
			})
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: joblifecycle.Namespace, Finalizers: []string{joblifecycle.CompletionFinalizer}},
				Spec: batchv1.JobSpec{BackoffLimit: ptr.To[int32](0), TTLSecondsAfterFinished: ptr.To[int32](1), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "check", Image: "busybox:1.37.0", Command: []string{"sh", "-c", `test "$PASSWORD" = expected`}, Env: []corev1.EnvVar{{Name: "PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: jobcredentials.Name(name)}, Key: "password"}}}}}},
				}}},
			}
			joblifecycle.Track(&job.ObjectMeta, parent, "VirtualMachineBackup")
			job, err = credentials.Create(job)
			require.NoError(t, err)
			require.NoError(t, credentials.Ensure(job, func() (map[string][]byte, error) { return map[string][]byte{"password": []byte("expected")}, nil }))
			require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
				current, err := jobs.Get(joblifecycle.Namespace, name, metav1.GetOptions{})
				return err == nil && current.Status.Succeeded > 0, err
			}))
			require.NoError(t, client.HarvesterhciV1beta1().VirtualMachineBackups(parent.Namespace).Delete(ctx, parent.Name, metav1.DeleteOptions{PropagationPolicy: &policy}))
			// Cross-namespace Jobs survive parent deletion until controller cleanup runs.
			retained, err := jobs.Get(joblifecycle.Namespace, name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Contains(t, retained.Finalizers, joblifecycle.CompletionFinalizer)
			require.NoError(t, joblifecycle.DeleteJobs(jobs, parent, "VirtualMachineBackup"))
			require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
				_, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(ctx, jobcredentials.Name(name), metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}))
		})
	}
}

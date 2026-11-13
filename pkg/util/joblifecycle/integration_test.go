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
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

// Run against a disposable cluster with the VMBackup and VMRestore CRDs installed:
// HARVESTER_CLEANUP_TEST_KUBECONFIG=... go test ./pkg/util/joblifecycle -run TestCleanupLifecycle -v
// This needs the real garbage collector and Job controller, not an envtest API server.
func TestCleanupLifecycle(t *testing.T) {
	kubeconfig := os.Getenv("HARVESTER_CLEANUP_TEST_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("set HARVESTER_CLEANUP_TEST_KUBECONFIG to a disposable cluster")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := versioned.NewForConfig(config)
	require.NoError(t, err)
	for _, policy := range []metav1.DeletionPropagation{
		metav1.DeletePropagationForeground, metav1.DeletePropagationBackground,
	} {
		t.Run(string(policy), func(t *testing.T) {
			runCleanup(t, client, policy)
		})
	}
}

func runCleanup(t *testing.T, client *versioned.Clientset, policy metav1.DeletionPropagation) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	namespace, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "backup-cleanup-"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		jobs, err := client.BatchV1().Jobs(namespace.Name).List(cleanupCtx, metav1.ListOptions{})
		require.NoError(t, err)
		for i := range jobs.Items {
			require.NoError(t, joblifecycle.Delete(fakeclients.JobClient(client.BatchV1().Jobs), &jobs.Items[i]))
		}
		backups := client.HarvesterhciV1beta1().VirtualMachineBackups(namespace.Name)
		if vmb, err := backups.Get(cleanupCtx, "backup", metav1.GetOptions{}); err == nil {
			vmb.Finalizers = nil
			_, _ = backups.Update(cleanupCtx, vmb, metav1.UpdateOptions{})
		}
		restores := client.HarvesterhciV1beta1().VirtualMachineRestores(namespace.Name)
		if vmr, err := restores.Get(cleanupCtx, "restore", metav1.GetOptions{}); err == nil {
			vmr.Finalizers = nil
			_, _ = restores.Update(cleanupCtx, vmr, metav1.UpdateOptions{})
		}
		require.NoError(t, client.CoreV1().Namespaces().Delete(
			cleanupCtx, namespace.Name, metav1.DeleteOptions{},
		))
	})
	vmb := createTestBackup(t, ctx, client, namespace.Name)
	vmb = deleteTestBackup(t, ctx, client, vmb, policy)
	names := []string{"forget-os", "forget-data"}
	for _, name := range names {
		_, err := client.BatchV1().Jobs(namespace.Name).Create(ctx, testJob(vmb, name), metav1.CreateOptions{})
		require.NoError(t, err)
	}
	runTestJobs(t, ctx, client, vmb, names)
	finishTestCleanup(t, ctx, client, vmb, names)
	testTTL(t, ctx, client, namespace.Name)
	testRestoreCancellation(t, ctx, client, namespace.Name, policy)
}

func testRestoreCancellation(
	t *testing.T, ctx context.Context, client *versioned.Clientset,
	namespace string, policy metav1.DeletionPropagation,
) {
	vmr := createTestRestore(t, ctx, client, namespace)
	job := testJob(&harvesterv1.VirtualMachineBackup{ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}, "restore")
	joblifecycle.Track(&job.ObjectMeta, vmr, "VirtualMachineRestore")
	job.Spec.TTLSecondsAfterFinished = ptr.To[int32](60)
	jobs := fakeclients.JobClient(client.BatchV1().Jobs)
	job, err := jobs.Create(job)
	require.NoError(t, err)
	deleteTestRestore(t, ctx, client, vmr, policy)
	job, err = jobs.Get(namespace, job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, joblifecycle.ValidateJob(vmr, "VirtualMachineRestore", job))
	require.Nil(t, job.DeletionTimestamp, "deleting the parent must not cancel the Job through GC")

	// The persisted deletion request allows cancellation without a source VMBackup.
	vmr, err = client.HarvesterhciV1beta1().VirtualMachineRestores(namespace).Get(ctx, vmr.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, vmr.DeletionTimestamp)
	require.NoError(t, joblifecycle.DeleteJobs(jobs, vmr, "VirtualMachineRestore"))
	finishTestRestore(t, ctx, client, vmr)
	poll(t, ctx, func() (bool, error) {
		_, err := jobs.Get(namespace, job.Name, metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	})
	poll(t, ctx, func() (bool, error) {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		return err == nil && len(pods.Items) == 0, err
	})
}

func createTestRestore(
	t *testing.T, ctx context.Context, client *versioned.Clientset, namespace string,
) *harvesterv1.VirtualMachineRestore {
	t.Helper()
	vmr, err := client.HarvesterhciV1beta1().VirtualMachineRestores(namespace).Create(
		ctx, &harvesterv1.VirtualMachineRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name: "restore", Namespace: namespace, Finalizers: []string{"test.harvesterhci.io/cleanup"},
			},
			Spec: harvesterv1.VirtualMachineRestoreSpec{
				VirtualMachineBackupName: "missing", VirtualMachineBackupNamespace: namespace,
				Target: corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("kubevirt.io"), Kind: "VirtualMachine", Name: "vm",
				},
			},
		}, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	return vmr
}

func deleteTestRestore(
	t *testing.T, ctx context.Context, client *versioned.Clientset,
	vmr *harvesterv1.VirtualMachineRestore, policy metav1.DeletionPropagation,
) {
	t.Helper()
	secrets := client.CoreV1().Secrets(vmr.Namespace)
	owned := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "restore-owned", Namespace: vmr.Namespace,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: harvesterv1.SchemeGroupVersion.String(), Kind: "VirtualMachineRestore",
			Name: vmr.Name, UID: vmr.UID, BlockOwnerDeletion: ptr.To(true),
		}},
	}}
	_, err := secrets.Create(ctx, owned, metav1.CreateOptions{})
	require.NoError(t, err)
	restores := client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace)
	require.NoError(t, restores.Delete(ctx, vmr.Name, metav1.DeleteOptions{PropagationPolicy: &policy}))
	if policy == metav1.DeletePropagationForeground {
		poll(t, ctx, func() (bool, error) {
			_, err := secrets.Get(ctx, owned.Name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	}
	current, err := restores.Get(ctx, vmr.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, current.DeletionTimestamp)
}

func finishTestRestore(
	t *testing.T, ctx context.Context, client *versioned.Clientset, vmr *harvesterv1.VirtualMachineRestore,
) {
	t.Helper()
	restores := client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace)
	poll(t, ctx, func() (bool, error) {
		current, err := restores.Get(ctx, vmr.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		current.Finalizers = nil
		_, err = restores.Update(ctx, current, metav1.UpdateOptions{})
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return err == nil, err
	})
	poll(t, ctx, func() (bool, error) {
		_, err := restores.Get(ctx, vmr.Name, metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	})
}

func createTestBackup(
	t *testing.T, ctx context.Context, client *versioned.Clientset, namespace string,
) *harvesterv1.VirtualMachineBackup {
	vmb, err := client.HarvesterhciV1beta1().VirtualMachineBackups(namespace).Create(
		ctx, &harvesterv1.VirtualMachineBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "backup", Namespace: namespace,
				Finalizers: []string{"test.harvesterhci.io/cleanup"},
			},
			Spec: harvesterv1.VirtualMachineBackupSpec{
				Type: harvesterv1.Kopia,
				Source: corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("kubevirt.io"), Kind: "VirtualMachine", Name: "vm",
				},
			},
		}, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	return vmb
}

func deleteTestBackup(
	t *testing.T, ctx context.Context, client *versioned.Clientset,
	vmb *harvesterv1.VirtualMachineBackup, policy metav1.DeletionPropagation,
) *harvesterv1.VirtualMachineBackup {
	// An owned sibling demonstrates that a parent's custom finalizer does not
	// protect dependents during foreground deletion.
	owned := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "owned-sibling", Namespace: vmb.Namespace}}
	owned.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: harvesterv1.SchemeGroupVersion.String(), Kind: "VirtualMachineBackup",
		Name: vmb.Name, UID: vmb.UID, BlockOwnerDeletion: ptr.To(true),
	}}
	secrets := client.CoreV1().Secrets(vmb.Namespace)
	_, err := secrets.Create(ctx, owned, metav1.CreateOptions{})
	require.NoError(t, err)
	backups := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace)
	require.NoError(t, backups.Delete(ctx, vmb.Name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}))
	if policy == metav1.DeletePropagationForeground {
		poll(t, ctx, func() (bool, error) {
			_, err := secrets.Get(ctx, owned.Name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	} else {
		_, err = secrets.Get(ctx, owned.Name, metav1.GetOptions{})
		require.NoError(t, err)
	}
	vmb, err = backups.Get(ctx, vmb.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, vmb.DeletionTimestamp)

	return vmb
}

func runTestJobs(
	t *testing.T, ctx context.Context, client *versioned.Clientset,
	vmb *harvesterv1.VirtualMachineBackup, names []string,
) {
	// Open the gates one volume at a time after the parent enters deletion.
	// A completed Job must retain its result while the other volume waits.
	_, err := client.CoreV1().ConfigMaps(vmb.Namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "start", Namespace: vmb.Namespace},
		Data:       map[string]string{},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	jobs := client.BatchV1().Jobs(vmb.Namespace)
	for index, name := range names {
		job, err := jobs.Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Nil(t, job.DeletionTimestamp)
		require.NoError(t, joblifecycle.ValidateJob(vmb, "VirtualMachineBackup", job))
		openTestGate(t, ctx, client, vmb.Namespace, name)
		poll(t, ctx, func() (bool, error) {
			job, err := jobs.Get(ctx, name, metav1.GetOptions{})
			return err == nil && job.Status.Succeeded == 1, err
		})
		if index == 0 {
			retainCompletedJob(t, ctx, client, vmb.Namespace, name)
		}
	}
}

func openTestGate(t *testing.T, ctx context.Context, client *versioned.Clientset, namespace, name string) {
	t.Helper()
	configMaps := client.CoreV1().ConfigMaps(namespace)
	gate, err := configMaps.Get(ctx, "start", metav1.GetOptions{})
	require.NoError(t, err)
	gate.Data[name] = "true"
	_, err = configMaps.Update(ctx, gate, metav1.UpdateOptions{})
	require.NoError(t, err)
}

// A deletion request cannot erase an earlier result while another volume waits.
func retainCompletedJob(t *testing.T, ctx context.Context, client *versioned.Clientset, namespace, name string) {
	t.Helper()
	jobs := client.BatchV1().Jobs(namespace)
	job, err := jobs.Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Nil(t, job.Spec.TTLSecondsAfterFinished)
	require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
	require.NoError(t, jobs.Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationForeground),
		Preconditions:     &metav1.Preconditions{UID: &job.UID},
	}))
	poll(t, ctx, func() (bool, error) {
		job, err := jobs.Get(ctx, name, metav1.GetOptions{})
		return err == nil && job.DeletionTimestamp != nil && job.Status.Succeeded == 1, err
	})
}

func finishTestCleanup(
	t *testing.T, ctx context.Context, client *versioned.Clientset,
	vmb *harvesterv1.VirtualMachineBackup, names []string,
) {
	backups := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace)
	// Fetch again because the garbage collector may have removed its own finalizer.
	vmb, err := backups.Get(ctx, vmb.Name, metav1.GetOptions{})
	require.NoError(t, err)
	vmbo := common.NewVMBackupOperatorBuilder().WithClient(
		fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups),
	).Build()
	vmb, err = common.Commit(vmbo, vmb, true)
	require.NoError(t, err)
	jobs := fakeclients.JobClient(client.BatchV1().Jobs)
	for _, name := range names {
		job, err := jobs.Get(vmb.Namespace, name, metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, joblifecycle.ValidateJob(vmb, "VirtualMachineBackup", job))
		require.NoError(t, joblifecycle.Delete(jobs, job))
	}
	// Finalize the VMBackup after requests succeed, without waiting for Job GC.
	vmb.Finalizers = nil
	_, err = backups.Update(ctx, vmb, metav1.UpdateOptions{})
	require.NoError(t, err)
	poll(t, ctx, func() (bool, error) {
		_, err := backups.Get(ctx, vmb.Name, metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	})
	for _, name := range names {
		poll(t, ctx, func() (bool, error) {
			_, err := jobs.Get(vmb.Namespace, name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	}
	poll(t, ctx, func() (bool, error) {
		pods, err := client.CoreV1().Pods(vmb.Namespace).List(ctx, metav1.ListOptions{})
		return err == nil && len(pods.Items) == 0, err
	})
}

func testJob(vmb *harvesterv1.VirtualMachineBackup, name string) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: vmb.Namespace,
			Finalizers: []string{joblifecycle.CompletionFinalizer},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To[int32](0),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				InitContainers: []corev1.Container{{
					Name: "gate", Image: "busybox:1.37.0", ImagePullPolicy: corev1.PullIfNotPresent,
					Command: []string{"sh", "-c", "true"},
					Env: []corev1.EnvVar{{Name: "READY", ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "start"}, Key: name,
						},
					}}},
				}},
				Containers: []corev1.Container{{
					Name: "forget", Image: "busybox:1.37.0", ImagePullPolicy: corev1.PullIfNotPresent,
					Command: []string{"sh", "-c", `test "$PASSWORD" = expected`},
					Env:     []corev1.EnvVar{{Name: "PASSWORD", Value: "expected"}},
				}},
			}},
		},
	}
	joblifecycle.Track(&job.ObjectMeta, vmb, "VirtualMachineBackup")
	return job
}

// TTL collects completed Jobs, with restore results retained until released.
func testTTL(t *testing.T, ctx context.Context, client *versioned.Clientset, namespace string) {
	jobs := fakeclients.JobClient(client.BatchV1().Jobs)
	for _, name := range []string{"backup-ttl", "restore-ttl"} {
		job := testJob(&harvesterv1.VirtualMachineBackup{ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}, name)
		job.Labels, job.Annotations = nil, nil
		if name == "backup-ttl" {
			job.Finalizers = nil
		}
		job.Spec.Template.Spec.InitContainers = nil
		job.Spec.TTLSecondsAfterFinished = ptr.To[int32](1)
		job, err := jobs.Create(job)
		require.NoError(t, err)
		if name == "restore-ttl" {
			poll(t, ctx, func() (bool, error) {
				job, err := jobs.Get(namespace, name, metav1.GetOptions{})
				return err == nil && job.Status.Succeeded == 1 && job.DeletionTimestamp != nil, err
			})
			require.NoError(t, joblifecycle.Release(jobs, job))
		}
		poll(t, ctx, func() (bool, error) {
			_, err := jobs.Get(namespace, name, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
		poll(t, ctx, func() (bool, error) {
			pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
			return err == nil && len(pods.Items) == 0, err
		})
	}
}

func poll(t *testing.T, ctx context.Context, condition func() (bool, error)) {
	t.Helper()
	require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true,
		func(context.Context) (bool, error) { return condition() }))
}

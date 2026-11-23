package datamover

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

// Run on a disposable CSI cluster with Harvester CRDs but without its controllers/webhooks.
// HARVESTER_TRANSFER_TEST_KUBECONFIG=... HARVESTER_TRANSFER_TEST_CLASSES='longhorn:longhorn,lvm:lvm' \
// go test ./pkg/util/datamover -run TestCSITransfer -v -timeout 30m
// Each pair names an existing StorageClass and VolumeSnapshotClass. Both Immediate
// and WaitForFirstConsumer classes can be included; the Jobs trigger provisioning.
func TestCSITransfer(t *testing.T) {
	path, classes := os.Getenv("HARVESTER_TRANSFER_TEST_KUBECONFIG"), os.Getenv("HARVESTER_TRANSFER_TEST_CLASSES")
	if path == "" || classes == "" {
		t.Skip("set transfer test kubeconfig and storage:snapshot class pairs")
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	require.NoError(t, err)
	kube, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	custom, err := versioned.NewForConfig(config)
	require.NoError(t, err)
	snapshotClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	snapshots := Snapshots{Client: snapshotClient}
	for _, pair := range strings.Split(classes, ",") {
		sc, vsc, ok := strings.Cut(pair, ":")
		require.True(t, ok, "use storageClass:snapshotClass")
		t.Run(sc, func(t *testing.T) { runCSITransfer(t, kube, custom, snapshots, sc, vsc) })
	}
}

func runCSITransfer(t *testing.T, kube kubernetes.Interface, custom *versioned.Clientset, snapshots Snapshots, sc, snapshotClass string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	namespace, err := kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "repository-csi-"}}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: joblifecycle.Namespace}}, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
	backup, err := custom.HarvesterhciV1beta1().VirtualMachineBackups(namespace.Name).Create(ctx, &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: namespace.Name},
		Spec:       harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic, Source: corev1.TypedLocalObjectReference{APIGroup: ptr.To("kubevirt.io"), Kind: "VirtualMachine", Name: "vm"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	restore, err := custom.HarvesterhciV1beta1().VirtualMachineRestores(namespace.Name).Create(ctx, &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: namespace.Name},
		Spec:       harvesterv1.VirtualMachineRestoreSpec{VirtualMachineBackupName: backup.Name, VirtualMachineBackupNamespace: backup.Namespace, Target: corev1.TypedLocalObjectReference{APIGroup: ptr.To("kubevirt.io"), Kind: "VirtualMachine", Name: "restored"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	alias := joblifecycle.Name("import", backup, "volume")
	stage := joblifecycle.Name("stage", restore, "volume")
	transferName := joblifecycle.Name("transfer", restore, "volume")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = wait.PollUntilContextCancel(cleanup, time.Second, true, func(context.Context) (bool, error) { return StopJobs(kube, restore, RestoreKind) == nil, nil })
		_ = wait.PollUntilContextCancel(cleanup, time.Second, true, func(context.Context) (bool, error) { return (Volumes{Client: kube}).Cleanup(restore) == nil, nil })
		_ = kube.BatchV1().Jobs(joblifecycle.Namespace).Delete(cleanup, alias, metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)})
		_ = wait.PollUntilContextCancel(cleanup, time.Second, true, func(context.Context) (bool, error) { return DeleteClaim(kube, backup, BackupKind, alias) == nil, nil })
		_ = wait.PollUntilContextCancel(cleanup, time.Second, true, func(context.Context) (bool, error) { return snapshots.Delete(backup, alias) == nil, nil })
		_ = kube.CoreV1().Namespaces().Delete(cleanup, namespace.Name, metav1.DeleteOptions{})
	})
	source := integrationClaim("source", namespace.Name, sc)
	_, err = kube.CoreV1().PersistentVolumeClaims(source.Namespace).Create(ctx, source, metav1.CreateOptions{})
	require.NoError(t, err)
	marker := "harvester-transfer"
	writer := integrationJob("write-source", source.Namespace, source.Name, fmt.Sprintf("printf %%s %s > /dev/volume; sync", marker))
	runIntegrationJob(t, ctx, kube, writer)
	deleteIntegrationJob(t, ctx, kube, writer.Namespace, writer.Name)
	original := &snapshotv1.VolumeSnapshot{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snapshot.storage.k8s.io/v1", Kind: "VolumeSnapshot"},
		ObjectMeta: metav1.ObjectMeta{Name: "original", Namespace: source.Namespace, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(backup, harvesterv1.SchemeGroupVersion.WithKind(BackupKind))}},
		Spec:       snapshotv1.VolumeSnapshotSpec{VolumeSnapshotClassName: &snapshotClass, Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: &source.Name}},
	}
	joblifecycle.Track(&original.ObjectMeta, backup, BackupKind)
	_, err = snapshots.create(snapshotResource, original.Namespace, original)
	require.NoError(t, err)
	require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
		err := snapshots.Import(backup, original.Name, alias)
		if err == ErrPending {
			return false, nil
		}
		return err == nil, err
	}))
	clone := integrationClaim(alias, joblifecycle.Namespace, sc)
	clone.Spec.DataSource = &corev1.TypedLocalObjectReference{APIGroup: ptr.To("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: alias}
	joblifecycle.Track(&clone.ObjectMeta, backup, BackupKind)
	_, err = kube.CoreV1().PersistentVolumeClaims(clone.Namespace).Create(ctx, clone, metav1.CreateOptions{})
	require.NoError(t, err)
	reader := integrationJob(alias, joblifecycle.Namespace, alias, fmt.Sprintf(`test "$(head -c %d /dev/volume)" = %s`, len(marker), marker))
	joblifecycle.Track(&reader.ObjectMeta, backup, BackupKind)
	runIntegrationJob(t, ctx, kube, reader)
	deleteIntegrationJob(t, ctx, kube, reader.Namespace, reader.Name)
	staging := integrationClaim(stage, joblifecycle.Namespace, sc)
	joblifecycle.Track(&staging.ObjectMeta, restore, RestoreKind)
	_, err = kube.CoreV1().PersistentVolumeClaims(staging.Namespace).Create(ctx, staging, metav1.CreateOptions{})
	require.NoError(t, err)
	job := integrationJob(transferName, joblifecycle.Namespace, stage, fmt.Sprintf("printf %%s %s > /dev/volume; sync", marker))
	joblifecycle.Track(&job.ObjectMeta, restore, RestoreKind)
	runIntegrationJob(t, ctx, kube, job)
	destination := integrationClaim("destination", namespace.Name, sc)
	volumes := Volumes{Client: kube}
	require.ErrorIs(t, volumes.BeginHandoff(restore, stage, transferName, destination), ErrPending)
	require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
		_, ready, err := (Volumes{Client: kube}).ResumeHandoff(restore, stage, destination)
		if err == ErrPending {
			return false, nil
		}
		return ready, err
	}))
	destinationReader := integrationJob("read-destination", destination.Namespace, destination.Name, reader.Spec.Template.Spec.Containers[0].Args[0])
	runIntegrationJob(t, ctx, kube, destinationReader)
	deleteIntegrationJob(t, ctx, kube, destinationReader.Namespace, destinationReader.Name)
}

func integrationClaim(name, namespace, sc string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &sc, VolumeMode: ptr.To(corev1.PersistentVolumeBlock), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
		},
	}
}

func integrationJob(name, namespace, claim, script string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: batchv1.JobSpec{BackoffLimit: ptr.To[int32](0), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: ptr.To(false),
			Containers: []corev1.Container{{Name: "io", Image: "busybox:1.37.0", Command: []string{"sh", "-ec"}, Args: []string{script}, VolumeDevices: []corev1.VolumeDevice{{Name: "volume", DevicePath: "/dev/volume"}}}},
			Volumes:    []corev1.Volume{{Name: "volume", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}}},
		}}},
	}
}

func runIntegrationJob(t *testing.T, ctx context.Context, kube kubernetes.Interface, job *batchv1.Job) {
	t.Helper()
	_, err := kube.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
		current, err := kube.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if current.Status.Failed > 0 {
			return false, fmt.Errorf("CSI test job %s/%s failed", job.Namespace, job.Name)
		}
		return current.Status.Succeeded > 0, nil
	}))
}

func deleteIntegrationJob(t *testing.T, ctx context.Context, kube kubernetes.Interface, namespace, name string) {
	t.Helper()
	require.NoError(t, kube.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}))
	require.NoError(t, wait.PollUntilContextCancel(ctx, time.Second, true, func(context.Context) (bool, error) {
		_, err := kube.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}))
}

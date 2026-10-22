package restic

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/settings"
)

func resetResticSettings(t *testing.T) {
	t.Helper()
	require.NoError(t, settings.ResticCacheSize.Set(settings.ResticCacheSize.GetDefault()))
	require.NoError(t, settings.ResticNoCache.Set(settings.ResticNoCache.GetDefault()))
	require.NoError(t, settings.ResticMaxConcurrentJobs.Set(settings.ResticMaxConcurrentJobs.GetDefault()))
	require.NoError(t, settings.ResticEphemeralRequest.Set(settings.ResticEphemeralRequest.GetDefault()))
	require.NoError(t, settings.ResticEphemeralLimit.Set(settings.ResticEphemeralLimit.GetDefault()))
	t.Cleanup(func() {
		_ = settings.ResticCacheSize.Set(settings.ResticCacheSize.GetDefault())
		_ = settings.ResticNoCache.Set(settings.ResticNoCache.GetDefault())
		_ = settings.ResticMaxConcurrentJobs.Set(settings.ResticMaxConcurrentJobs.GetDefault())
		_ = settings.ResticEphemeralRequest.Set(settings.ResticEphemeralRequest.GetDefault())
		_ = settings.ResticEphemeralLimit.Set(settings.ResticEphemeralLimit.GetDefault())
	})
}

func TestCacheVolumeAndMount(t *testing.T) {
	resetResticSettings(t)
	require.NoError(t, settings.ResticCacheSize.Set("2Gi"))

	volume, mount, err := CacheVolumeAndMount()
	require.NoError(t, err)
	require.NotNil(t, volume)
	require.NotNil(t, mount)
	require.Equal(t, CacheVolume, volume.Name)
	require.NotNil(t, volume.EmptyDir)
	require.Equal(t, resource.MustParse("2Gi"), *volume.EmptyDir.SizeLimit)
	require.Equal(t, CacheVolume, mount.Name)
	require.Equal(t, CacheDir, mount.MountPath)
}

func TestCacheVolumeAndMountDisabledByNoCache(t *testing.T) {
	resetResticSettings(t)
	require.NoError(t, settings.ResticNoCache.Set("true"))

	volume, mount, err := CacheVolumeAndMount()
	require.NoError(t, err)
	require.Nil(t, volume)
	require.Nil(t, mount)
	require.Equal(t, "restic --no-cache", Command())
}

func TestRepositoryAndRuntimeCommandHonorBucketRegion(t *testing.T) {
	resetResticSettings(t)
	repository, err := Repository(&settings.BackupTarget{
		Type:         settings.S3BackupType,
		Endpoint:     "http://10.115.54.34:9000",
		BucketName:   "mybucket",
		BucketRegion: "pcloud",
	})
	require.NoError(t, err)
	require.Equal(t, "s3:http://10.115.54.34:9000/mybucket/restic", repository.URL)
	require.Equal(t, "pcloud", repository.Region)
	require.Contains(t, Env("credentials", repository), corev1.EnvVar{Name: RegionKey, Value: "pcloud"})
	require.Equal(t, `restic -o s3.region="$RESTIC_S3_REGION"`, command(repository.Region))
}

func TestRepositoryUsesAWSDefaultEndpoint(t *testing.T) {
	repository, err := Repository(&settings.BackupTarget{
		Type:         settings.S3BackupType,
		BucketName:   "mybucket",
		BucketRegion: "us-east-1",
	})
	require.NoError(t, err)
	require.Equal(t, "s3:s3.amazonaws.com/mybucket/restic", repository.URL)
	require.Equal(t, "us-east-1", repository.Region)
}

func TestRuntimeCommandOmitsEmptyBucketRegion(t *testing.T) {
	resetResticSettings(t)
	repository, err := Repository(&settings.BackupTarget{
		Type:       settings.S3BackupType,
		Endpoint:   "http://10.115.54.34:9000",
		BucketName: "mybucket",
	})
	require.NoError(t, err)
	require.Empty(t, repository.Region)
	require.NotContains(t, Env("credentials", repository), corev1.EnvVar{Name: RegionKey})
	require.Equal(t, "restic", command(repository.Region))
}

func TestJobHasCapacityReturnsFalseWhenGlobalLimitIsReached(t *testing.T) {
	resetResticSettings(t)
	require.NoError(t, settings.ResticMaxConcurrentJobs.Set("3"))
	clientset := fake.NewSimpleClientset(
		resticPod("pod1", "node1", corev1.PodRunning),
		resticPod("pod2", "node1", corev1.PodRunning),
		resticPod("pod3", "node2", corev1.PodRunning),
	)

	hasCapacity, err := JobHasCapacity(context.Background(), clientset)
	require.NoError(t, err)
	require.False(t, hasCapacity)
}

func TestJobHasCapacityReturnsTrueBelowGlobalLimit(t *testing.T) {
	resetResticSettings(t)
	require.NoError(t, settings.ResticMaxConcurrentJobs.Set("3"))
	clientset := fake.NewSimpleClientset(
		resticPod("pod1", "node1", corev1.PodRunning),
		resticPod("pod2", "node1", corev1.PodSucceeded),
	)

	hasCapacity, err := JobHasCapacity(context.Background(), clientset)
	require.NoError(t, err)
	require.True(t, hasCapacity)
}

func TestSnapshotCheckJobResultReportsFound(t *testing.T) {
	job := snapshotCheckJob("check", 1, 0)

	result, err := SnapshotCheckJobResult(context.Background(), fake.NewSimpleClientset(), job)
	require.NoError(t, err)
	require.Equal(t, SnapshotCheckFound, result)
}

func TestSnapshotCheckJobResultReportsMissing(t *testing.T) {
	job := snapshotCheckJob("check", 0, 1)
	clientset := fake.NewSimpleClientset(snapshotCheckPod("check-pod", job.Name, SnapshotCheckMissingExitCode))

	result, err := SnapshotCheckJobResult(context.Background(), clientset, job)
	require.NoError(t, err)
	require.Equal(t, SnapshotCheckMissing, result)
}

func TestSnapshotCheckJobResultReportsFailed(t *testing.T) {
	job := snapshotCheckJob("check", 0, 1)
	clientset := fake.NewSimpleClientset(snapshotCheckPod("check-pod", job.Name, 1))

	result, err := SnapshotCheckJobResult(context.Background(), clientset, job)
	require.NoError(t, err)
	require.Equal(t, SnapshotCheckFailed, result)
}

func TestForgetCommand(t *testing.T) {
	tags := []string{NamespaceTag("default"), VMBackupTag("backup"), SnapshotTag("volume")}
	command := ForgetCommand("restic --no-cache", tags)
	require.Contains(t, command, "restic --no-cache --retry-lock 5m forget")
	require.Contains(t, command, "--tag=ns=default,vmb=backup,sn=volume")
	require.Contains(t, command, "--unsafe-allow-remove-all --prune")
	require.NotContains(t, command, "unlock")
	require.NotContains(t, command, "snapshots > /dev/null")
}

func TestForgetCommandExitCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{name: "success"},
		{name: "missing repository", code: 10},
		{name: "lock wait exhausted", code: 11, want: 11},
		{name: "invalid credentials", code: 12, want: 12},
		{name: "other backend failure", code: 1, want: 1},
		{name: "interrupted", code: 130, want: 130},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A shell function replaces only the Restic binary; execute the same
			// wrapper used by forget Jobs to check that it preserves failures.
			script := fmt.Sprintf("restic() { return %d; }; ", tt.code) +
				ForgetCommand("restic", []string{"ns=default", "vmb=backup", "sn=volume"})
			err := exec.Command("/bin/sh", "-c", script).Run()
			if tt.want == 0 {
				require.NoError(t, err)
				return
			}
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, tt.want, exitErr.ExitCode())
		})
	}
}

func resticPod(name, nodeName string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				LabelResticJob: LabelValueTrue,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

func snapshotCheckJob(name string, succeeded, failed int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Succeeded: succeeded,
			Failed:    failed,
		},
	}
}

func snapshotCheckPod(name, jobName string, exitCode int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"job-name": jobName,
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: SnapshotCheckContainerName,
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode},
				},
			}},
		},
	}
}

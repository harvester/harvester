package kopia

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/settings"
)

func resetKopiaSettings(t *testing.T) {
	t.Helper()
	require.NoError(t, settings.KopiaCacheSize.Set(settings.KopiaCacheSize.GetDefault()))
	require.NoError(t, settings.KopiaMaxConcurrentJobs.Set(settings.KopiaMaxConcurrentJobs.GetDefault()))
	require.NoError(t, settings.KopiaEphemeralRequest.Set(settings.KopiaEphemeralRequest.GetDefault()))
	require.NoError(t, settings.KopiaEphemeralLimit.Set(settings.KopiaEphemeralLimit.GetDefault()))
	t.Cleanup(func() {
		_ = settings.KopiaCacheSize.Set(settings.KopiaCacheSize.GetDefault())
		_ = settings.KopiaMaxConcurrentJobs.Set(settings.KopiaMaxConcurrentJobs.GetDefault())
		_ = settings.KopiaEphemeralRequest.Set(settings.KopiaEphemeralRequest.GetDefault())
		_ = settings.KopiaEphemeralLimit.Set(settings.KopiaEphemeralLimit.GetDefault())
	})
}

func TestJobHasCapacityReturnsFalseWhenGlobalLimitIsReached(t *testing.T) {
	resetKopiaSettings(t)
	require.NoError(t, settings.KopiaMaxConcurrentJobs.Set("2"))
	clientset := fake.NewSimpleClientset(
		kopiaPod("pod1", corev1.PodRunning),
		kopiaPod("pod2", corev1.PodPending),
	)

	hasCapacity, err := JobHasCapacity(context.Background(), clientset)
	require.NoError(t, err)
	require.False(t, hasCapacity)
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

func TestS3RepositoryNormalizesHTTPEndpoint(t *testing.T) {
	repository, err := S3Repository(&settings.BackupTarget{
		Type:         settings.S3BackupType,
		Endpoint:     "http://10.115.54.34:9000",
		BucketName:   "mybucket",
		BucketRegion: "pcloud",
	})
	require.NoError(t, err)
	require.Equal(t, "10.115.54.34:9000", repository.Endpoint)
	require.Equal(t, "pcloud", repository.Region)
	require.Equal(t, "kopia/", repository.Prefix)
	require.True(t, repository.DisableTLS)
	require.Contains(t, Env("credentials", repository), corev1.EnvVar{Name: RegionKey, Value: "pcloud"})
	require.Contains(t, Env("credentials", repository), corev1.EnvVar{Name: PrefixKey, Value: "kopia/"})
	require.Contains(t, ConnectCommand(), `--region="$KOPIA_REGION"`)
	require.Contains(t, ConnectCommand(), `--prefix="$KOPIA_PREFIX"`)
	require.Contains(t, ConnectCommand(), `--content-cache-size-limit-mb="$KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"`)
	require.Contains(t, ConnectCommand(), `--metadata-cache-size-limit-mb="$KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"`)
	require.Contains(t, ConnectOrCreateCommand(), `--region="$KOPIA_REGION"`)
	require.Contains(t, ConnectOrCreateCommand(), `--prefix="$KOPIA_PREFIX"`)
	require.Contains(t, ConnectOrCreateCommand(), `--content-cache-size-limit-mb="$KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"`)
	require.Contains(t, ConnectOrCreateCommand(), `--metadata-cache-size-limit-mb="$KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"`)
	require.Contains(t, ConnectOrCreateCommand(), "/usr/bin/harvester kopia-prepare")
}

func TestS3RepositoryUsesAWSDefaultEndpoint(t *testing.T) {
	repository, err := S3Repository(&settings.BackupTarget{
		Type:         settings.S3BackupType,
		BucketName:   "mybucket",
		BucketRegion: "us-east-1",
	})
	require.NoError(t, err)
	require.Equal(t, "s3.amazonaws.com", repository.Endpoint)
	require.Equal(t, "us-east-1", repository.Region)
	require.False(t, repository.DisableTLS)
}

func TestCacheConfigEnvReservesCapacityForOtherEphemeralData(t *testing.T) {
	resetKopiaSettings(t)
	require.NoError(t, settings.KopiaCacheSize.Set("2Gi"))

	env, err := CacheConfigEnv()
	require.NoError(t, err)
	require.Contains(t, env, corev1.EnvVar{Name: ContentCacheSizeMBKey, Value: "256"})
	require.Contains(t, env, corev1.EnvVar{Name: ContentCacheSizeLimitMBKey, Value: "512"})
	require.Contains(t, env, corev1.EnvVar{Name: MetadataCacheSizeMBKey, Value: "256"})
	require.Contains(t, env, corev1.EnvVar{Name: MetadataCacheSizeLimitMBKey, Value: "512"})
}

func TestCacheConfigEnvRejectsTooSmallCache(t *testing.T) {
	resetKopiaSettings(t)
	require.NoError(t, settings.KopiaCacheSize.Set("4Mi"))

	_, err := CacheConfigEnv()
	require.ErrorContains(t, err, "must be at least 8Mi")
}

func TestS3RepositoryNormalizesHTTPSEndpoint(t *testing.T) {
	repository, err := S3Repository(&settings.BackupTarget{
		Type:       settings.S3BackupType,
		Endpoint:   "https://s3.example.com",
		BucketName: "mybucket",
	})
	require.NoError(t, err)
	require.Equal(t, "s3.example.com", repository.Endpoint)
	require.False(t, repository.DisableTLS)
}

func TestS3RepositoryRejectsEndpointPath(t *testing.T) {
	_, err := S3Repository(&settings.BackupTarget{
		Type:       settings.S3BackupType,
		Endpoint:   "http://10.115.54.34:9000/minio",
		BucketName: "mybucket",
	})
	require.ErrorContains(t, err, "must not include a path")
}

func TestSnapshotCommandsUseRepeatedTagFlags(t *testing.T) {
	tags := []string{"ns:default", "vmb:backup", "sn:snapshot"}
	wantFlags := "--tags 'ns:default' --tags 'vmb:backup' --tags 'sn:snapshot'"

	require.Equal(t, wantFlags, SnapshotTagsFlags(tags))
	require.Contains(t, SnapshotListCommand(tags), wantFlags)
	require.Contains(t, SnapshotCheckCommand("connect", tags), wantFlags)
	require.Contains(t, SnapshotDeleteCommand("connect", tags), wantFlags)
	require.Contains(t, SnapshotObjectCommand("connect", tags), wantFlags)
	require.NotContains(t, SnapshotListCommand(tags), "ns:default,vmb:backup")
	require.NotContains(t, SnapshotListCommand(tags), "/pv-name")
}

func kopiaPod(name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				LabelKopiaJob: LabelValueTrue,
			},
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

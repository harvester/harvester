package restic

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/snapshotcheck"
)

const (
	ImageEnvVar = "HARVESTER_IMAGE"

	PasswordKey = "RESTIC_PASSWORD"
	RegionKey   = "RESTIC_S3_REGION"

	CacheDirEnvVar    = "RESTIC_CACHE_DIR"
	CacheDir          = "/restic-cache"
	CacheVolume       = "restic-cache"
	defaultS3Endpoint = "s3.amazonaws.com"

	// repoSubpath isolates restic data under a dedicated prefix inside the
	// shared backup bucket so it doesn't collide with Longhorn / Kopia data.
	repoSubpath = "restic"

	LabelResticJob = "harvesterhci.io/restic-job"
	LabelValueTrue = "true"

	SnapshotCheckContainerName   = "check"
	SnapshotCheckMissingExitCode = snapshotcheck.MissingExitCode
)

type SnapshotCheckResult = snapshotcheck.Result

const (
	SnapshotCheckPending = snapshotcheck.Pending
	SnapshotCheckFound   = snapshotcheck.Found
	SnapshotCheckMissing = snapshotcheck.Missing
	SnapshotCheckFailed  = snapshotcheck.Failed
)

type JobRuntime struct {
	Image        string
	Labels       map[string]string
	Env          []corev1.EnvVar
	Resources    corev1.ResourceRequirements
	VolumeMounts []corev1.VolumeMount
	Volumes      []corev1.Volume
	Command      string
}

type SnapshotCheckJobOptions struct {
	Name            string
	Namespace       string
	Labels          map[string]string
	OwnerReferences []metav1.OwnerReference
	Runtime         *JobRuntime
	Tags            []string
}

// Image returns the harvester container image used for restic backup/restore
// jobs. The image must be injected via HARVESTER_IMAGE (set on the apiserver
// Deployment) so jobs run the same build as the controller. We refuse to fall
// back to a hardcoded default — a stale default is easy to ship by accident.
func Image() (string, error) {
	image := os.Getenv(ImageEnvVar)
	if image == "" {
		return "", fmt.Errorf("%s environment variable is not set", ImageEnvVar)
	}
	return image, nil
}

type RepositoryConfig struct {
	URL    string
	Region string
}

func RepositoryFromSetting() (*RepositoryConfig, error) {
	target, err := backuputil.CurrentTarget()
	if err != nil {
		return nil, err
	}
	return Repository(target)
}

func Repository(target *settings.BackupTarget) (*RepositoryConfig, error) {
	if target == nil {
		return nil, fmt.Errorf("backup target is nil")
	}

	switch target.Type {
	case settings.S3BackupType:
		endpoint := strings.TrimRight(target.Endpoint, "/")
		if endpoint == "" {
			endpoint = defaultS3Endpoint
		}
		if target.BucketName == "" {
			return nil, fmt.Errorf("s3 backup target bucket name is empty")
		}
		return &RepositoryConfig{
			URL:    fmt.Sprintf("s3:%s/%s/%s", endpoint, target.BucketName, repoSubpath),
			Region: target.BucketRegion,
		}, nil
	default:
		return nil, fmt.Errorf("restic engine currently supports %s backup targets only, got %s", settings.S3BackupType, target.Type)
	}
}

func Env(secretName string, repository *RepositoryConfig) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name: util.AWSAccessKey,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  util.AWSAccessKey,
			}},
		},
		{
			Name: util.AWSSecretKey,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  util.AWSSecretKey,
			}},
		},
		{Name: "RESTIC_REPOSITORY", Value: repository.URL},
		{
			Name: "RESTIC_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  PasswordKey,
			}},
		},
	}
	if repository.Region != "" {
		env = append(env, corev1.EnvVar{Name: RegionKey, Value: repository.Region})
	}
	if !NoCache() {
		env = append(env, corev1.EnvVar{Name: CacheDirEnvVar, Value: CacheDir})
	}
	return env
}

func Command() string {
	return command("")
}

func command(region string) string {
	command := "restic"
	if NoCache() {
		command += " --no-cache"
	}
	if region != "" {
		command += fmt.Sprintf(" -o s3.region=\"$%s\"", RegionKey)
	}
	return command
}

func NoCache() bool {
	enabled, err := strconv.ParseBool(settings.ResticNoCache.Get())
	if err == nil {
		return enabled
	}
	enabled, _ = strconv.ParseBool(settings.ResticNoCache.GetDefault())
	return enabled
}

func JobLabels(labels map[string]string) map[string]string {
	result := map[string]string{
		LabelResticJob: LabelValueTrue,
	}
	for k, v := range labels {
		result[k] = v
	}
	return result
}

func NewJobRuntime(
	ctx context.Context,
	clientset kubernetes.Interface,
	secretName string,
	repository *RepositoryConfig,
	labels map[string]string,
) (*JobRuntime, bool, error) {
	if repository == nil {
		return nil, false, fmt.Errorf("restic repository config is nil")
	}
	image, err := Image()
	if err != nil {
		return nil, false, err
	}
	hasCapacity, err := JobHasCapacity(ctx, clientset)
	if err != nil {
		return nil, false, err
	}
	if !hasCapacity {
		return nil, false, nil
	}
	cacheVolume, cacheMount, err := CacheVolumeAndMount()
	if err != nil {
		return nil, false, err
	}
	resources, err := JobResources()
	if err != nil {
		return nil, false, err
	}

	runtime := &JobRuntime{
		Image:     image,
		Labels:    labels,
		Env:       Env(secretName, repository),
		Resources: resources,
		Command:   command(repository.Region),
	}
	if cacheVolume != nil {
		runtime.Volumes = append(runtime.Volumes, *cacheVolume)
	}
	if cacheMount != nil {
		runtime.VolumeMounts = append(runtime.VolumeMounts, *cacheMount)
	}
	return runtime, true, nil
}

func CheckSnapshotJob(
	ctx context.Context,
	clientset kubernetes.Interface,
	jobName string,
	getJob func(name string) (*batchv1.Job, error),
	createJob func(name string) error,
) (SnapshotCheckResult, error) {
	return snapshotcheck.CheckJob(ctx, clientset, snapshotcheck.CheckJobOptions{
		EngineName:      "restic",
		ContainerName:   SnapshotCheckContainerName,
		MissingExitCode: SnapshotCheckMissingExitCode,
		JobName:         jobName,
		GetJob:          getJob,
		CreateJob:       createJob,
	})
}

func SnapshotCheckJobResult(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (SnapshotCheckResult, error) {
	return snapshotcheck.JobResult(ctx, clientset, job, "restic", SnapshotCheckContainerName, SnapshotCheckMissingExitCode)
}

func SnapshotCheckJobExitCode(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (int32, bool, error) {
	return snapshotcheck.JobExitCode(ctx, clientset, job, "restic", SnapshotCheckContainerName)
}

func NewSnapshotCheckJob(opts SnapshotCheckJobOptions) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            opts.Name,
			Namespace:       opts.Namespace,
			Labels:          opts.Labels,
			OwnerReferences: opts.OwnerReferences,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](0),
			TTLSecondsAfterFinished: ptr.To[int32](60),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: opts.Labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         SnapshotCheckContainerName,
						Image:        opts.Runtime.Image,
						Env:          opts.Runtime.Env,
						Command:      []string{"/bin/sh", "-c"},
						Resources:    opts.Runtime.Resources,
						VolumeMounts: opts.Runtime.VolumeMounts,
						Args:         []string{SnapshotCheckCommand(opts.Runtime.Command, opts.Tags)},
					}},
					Volumes: opts.Runtime.Volumes,
				},
			},
		},
	}
}

func SnapshotCheckCommand(resticCommand string, tags []string) string {
	return fmt.Sprintf(
		"set -eo pipefail; "+
			"SNAPSHOTS=$(%s snapshots --json --tag=%s | tr -d '[:space:]'); "+
			"test \"$SNAPSHOTS\" != \"[]\" || exit %d",
		resticCommand,
		strings.Join(tags, ","),
		SnapshotCheckMissingExitCode,
	)
}

// ForgetCommand waits for competing repository operations instead of failing
// immediately on an exclusive lock. Restic 0.17+ uses exit code 10 for a missing
// repository: only that case means there is nothing to clean up. Authentication,
// network and lock-timeout errors must remain failures, not permit finalization.
func ForgetCommand(resticCommand string, tags []string) string {
	// A single comma-separated --tag filter ANDs namespace, VMBackup and volume
	// tags. --unsafe-allow-remove-all explicitly removes all matching snapshots;
	// --keep-last 0 would instead be interpreted as no retention policy.
	return fmt.Sprintf(
		"%s --retry-lock 5m forget --tag=%s --unsafe-allow-remove-all --prune; "+
			"RESTIC_EXIT_CODE=$?; test \"$RESTIC_EXIT_CODE\" -ne 10 || exit 0; exit \"$RESTIC_EXIT_CODE\"",
		resticCommand,
		strings.Join(tags, ","),
	)
}

func (r *JobRuntime) VolumesWith(volumes ...corev1.Volume) []corev1.Volume {
	result := append([]corev1.Volume{}, r.Volumes...)
	return append(result, volumes...)
}

func CacheVolumeAndMount() (*corev1.Volume, *corev1.VolumeMount, error) {
	if NoCache() {
		return nil, nil, nil
	}
	sizeLimit, err := resource.ParseQuantity(settings.ResticCacheSize.Get())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse setting %s=%q: %w",
			settings.ResticCacheSizeSettingName, settings.ResticCacheSize.Get(), err)
	}
	return &corev1.Volume{
			Name: CacheVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &sizeLimit,
				},
			},
		}, &corev1.VolumeMount{
			Name:      CacheVolume,
			MountPath: CacheDir,
		}, nil
}

func JobResources() (corev1.ResourceRequirements, error) {
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	if err := addQuantity(resources.Requests, corev1.ResourceEphemeralStorage, settings.ResticEphemeralRequest); err != nil {
		return corev1.ResourceRequirements{}, err
	}
	if err := addQuantity(resources.Limits, corev1.ResourceEphemeralStorage, settings.ResticEphemeralLimit); err != nil {
		return corev1.ResourceRequirements{}, err
	}
	if len(resources.Requests) == 0 {
		resources.Requests = nil
	}
	if len(resources.Limits) == 0 {
		resources.Limits = nil
	}
	return resources, nil
}

func addQuantity(list corev1.ResourceList, name corev1.ResourceName, setting settings.Setting) error {
	value := setting.Get()
	if value == "" {
		return nil
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return fmt.Errorf("failed to parse setting %s=%q: %w", setting.Name, value, err)
	}
	list[name] = quantity
	return nil
}

func JobHasCapacity(ctx context.Context, clientset kubernetes.Interface) (bool, error) {
	maxConcurrent := settings.ResticMaxConcurrentJobs.GetInt()
	if maxConcurrent <= 0 {
		return true, nil
	}
	if clientset == nil {
		return false, fmt.Errorf("clientset is nil")
	}

	pods, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", LabelResticJob, LabelValueTrue),
	})
	if err != nil {
		return false, fmt.Errorf("listing restic pods for scheduling: %w", err)
	}

	activeTotal := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		activeTotal++
	}
	if activeTotal >= maxConcurrent {
		return false, nil
	}
	return true, nil
}

func SnapshotTag(backupName string) string {
	return fmt.Sprintf("sn=%s", backupName)
}

func NamespaceTag(namespace string) string {
	return fmt.Sprintf("ns=%s", namespace)
}

func VMBackupTag(vmBackupName string) string {
	return fmt.Sprintf("vmb=%s", vmBackupName)
}

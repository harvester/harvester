package kopia

import (
	"context"
	"fmt"
	"net/url"
	"os"
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

	PasswordKey       = "KOPIA_PASSWORD"
	BucketKey         = "KOPIA_BUCKET"
	EndpointKey       = "KOPIA_ENDPOINT"
	RegionKey         = "KOPIA_REGION"
	PrefixKey         = "KOPIA_PREFIX"
	DisableTLSKey     = "KOPIA_DISABLE_TLS"
	ConfigPath        = "/tmp/kopia.config"
	RepositoryPrefix  = "kopia/"
	defaultS3Endpoint = "s3.amazonaws.com"

	CacheDirEnvVar = "KOPIA_CACHE_DIRECTORY"
	CacheDir       = "/kopia-cache"
	CacheVolume    = "kopia-cache"
	LogDirEnvVar   = "KOPIA_LOG_DIR"

	ContentCacheSizeMBKey          = "KOPIA_CONTENT_CACHE_SIZE_MB"
	ContentCacheSizeLimitMBKey     = "KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"
	MetadataCacheSizeMBKey         = "KOPIA_METADATA_CACHE_SIZE_MB"
	MetadataCacheSizeLimitMBKey    = "KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"
	cacheCapacityReservedFraction  = int64(2)
	cacheHardLimitShareDenominator = int64(4)
	bytesPerMiB                    = int64(1024 * 1024)

	LabelKopiaJob         = "harvesterhci.io/kopia-job"
	LabelValueTrue        = "true"
	LabelKopiaMaintenance = "harvesterhci.io/kopia-maintenance"
	LabelMaintenanceType  = "harvesterhci.io/kopia-maintenance-type"

	QuickMaintenanceCronJobName = "kopia-maintenance-quick"
	FullMaintenanceCronJobName  = "kopia-maintenance-full"
	QuickMaintenanceSchedule    = "0 * * * *"
	FullMaintenanceSchedule     = "30 0 * * *"
	CheckForUpdatesEnvVar       = "KOPIA_CHECK_FOR_UPDATES"

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
	ConnectCommand  string
}

// Image returns the harvester container image used for kopia backup/restore
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

type Repository struct {
	Bucket     string
	Endpoint   string
	Region     string
	Prefix     string
	DisableTLS bool
}

func S3Repository(target *settings.BackupTarget) (*Repository, error) {
	if target == nil {
		return nil, fmt.Errorf("backup target is nil")
	}

	switch target.Type {
	case settings.S3BackupType:
		endpoint, disableTLS, err := normalizeS3Endpoint(target.Endpoint)
		if err != nil {
			return nil, err
		}
		if endpoint == "" {
			endpoint = defaultS3Endpoint
		}
		if target.BucketName == "" {
			return nil, fmt.Errorf("s3 backup target bucket name is empty")
		}
		return &Repository{
			Bucket:     target.BucketName,
			Endpoint:   endpoint,
			Region:     target.BucketRegion,
			Prefix:     RepositoryPrefix,
			DisableTLS: disableTLS,
		}, nil
	default:
		return nil, fmt.Errorf("kopia engine currently supports %s backup targets only, got %s", settings.S3BackupType, target.Type)
	}
}

func normalizeS3Endpoint(rawEndpoint string) (string, bool, error) {
	rawEndpoint = strings.TrimRight(rawEndpoint, "/")
	if rawEndpoint == "" {
		return "", false, nil
	}
	if !strings.Contains(rawEndpoint, "://") {
		return rawEndpoint, false, nil
	}

	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return "", false, fmt.Errorf("failed to parse s3 backup target endpoint %q: %w", rawEndpoint, err)
	}
	if parsed.Host == "" {
		return "", false, fmt.Errorf("s3 backup target endpoint %q is missing host", rawEndpoint)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false, fmt.Errorf("kopia s3 endpoint must not include a path, got %q", rawEndpoint)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("kopia s3 endpoint must not include query or fragment, got %q", rawEndpoint)
	}

	switch parsed.Scheme {
	case "http":
		return parsed.Host, true, nil
	case "https":
		return parsed.Host, false, nil
	default:
		return "", false, fmt.Errorf("unsupported s3 backup target endpoint scheme %q", parsed.Scheme)
	}
}

func RepositoryFromSetting() (*Repository, error) {
	target, err := backuputil.CurrentTarget()
	if err != nil {
		return nil, err
	}
	return S3Repository(target)
}

func Env(secretName string, repository *Repository) []corev1.EnvVar {
	return EnvWithPasswordKey(secretName, PasswordKey, repository)
}

// EnvWithPasswordKey returns the common Kopia repository environment while
// allowing callers to select the key containing the repository password. The
// backup and restore engines use their per-operation KOPIA_PASSWORD key, while
// maintenance uses the S3 secret-access-key directly, matching the password
// derivation used by those engines.
func EnvWithPasswordKey(secretName, passwordKey string, repository *Repository) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: CacheDirEnvVar, Value: CacheDir},
		{Name: LogDirEnvVar, Value: CacheDir},
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
		{
			Name: PasswordKey,
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  passwordKey,
			}},
		},
		{Name: BucketKey, Value: repository.Bucket},
		{Name: EndpointKey, Value: repository.Endpoint},
		{Name: RegionKey, Value: repository.Region},
		{Name: PrefixKey, Value: repository.Prefix},
		{Name: DisableTLSKey, Value: fmt.Sprintf("%t", repository.DisableTLS)},
	}
}

// MaintenanceCommand connects to (or initializes) the repository, takes
// maintenance ownership for the short-lived CronJob pod, and runs one explicit
// maintenance cycle. Automatic maintenance is disabled for the last two
// commands so taking ownership cannot trigger an additional implicit cycle.
func MaintenanceCommand(full bool) string {
	fullFlag := ""
	if full {
		fullFlag = " --full"
	}
	return fmt.Sprintf(
		"set -eo pipefail\n"+
			"%s\n"+
			"%s --no-auto-maintenance maintenance set --owner=me\n"+
			"%s --no-auto-maintenance maintenance run%s",
		ConnectOrCreateCommand(), Command(), Command(), fullFlag,
	)
}

// NewMaintenanceCronJob builds a short-lived repository maintenance job using
// the same image, S3 configuration, cache limits, and resource settings as the
// Kopia backup and restore engines.
func NewMaintenanceCronJob(name, schedule string, full bool, secretName string, repository *Repository) (*batchv1.CronJob, error) {
	image, err := Image()
	if err != nil {
		return nil, err
	}
	cacheVolume, cacheMount, err := CacheVolumeAndMount()
	if err != nil {
		return nil, err
	}
	cacheEnv, err := CacheConfigEnv()
	if err != nil {
		return nil, err
	}
	resources, err := JobResources()
	if err != nil {
		return nil, err
	}

	maintenanceType := "quick"
	if full {
		maintenanceType = "full"
	}
	labels := JobLabels(map[string]string{
		LabelKopiaMaintenance: LabelValueTrue,
		LabelMaintenanceType:  maintenanceType,
	})
	env := append(EnvWithPasswordKey(secretName, PasswordKey, repository), cacheEnv...)
	env = append(env, corev1.EnvVar{Name: CheckForUpdatesEnvVar, Value: "false"})

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: util.LonghornSystemNamespaceName,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: ptr.To[int32](1),
			FailedJobsHistoryLimit:     ptr.To[int32](1),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit:            ptr.To[int32](0),
					TTLSecondsAfterFinished: ptr.To[int32](3600),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{{
								Name:         "maintenance",
								Image:        image,
								Env:          env,
								Command:      []string{"/bin/sh", "-c"},
								Args:         []string{MaintenanceCommand(full)},
								Resources:    resources,
								VolumeMounts: []corev1.VolumeMount{*cacheMount},
							}},
							Volumes: []corev1.Volume{*cacheVolume},
						},
					},
				},
			},
		},
	}
	return cronJob, nil
}

func Command() string {
	return fmt.Sprintf("kopia --config-file=%s", ConfigPath)
}

func ConnectOrCreateCommand() string {
	connect := ConnectCommand()
	create := repositoryCommand("create")
	prepare := "/usr/bin/harvester " + PrepareSubcommand

	return fmt.Sprintf(
		"(%s || { (%s && %s) || %s; })",
		connect,
		prepare,
		create,
		connect,
	)
}

func ConnectCommand() string {
	return repositoryCommand("connect")
}

func repositoryCommand(action string) string {
	return fmt.Sprintf(
		"%[1]s repository %[2]s s3"+
			"$(if [ \"$%[8]s\" = \"true\" ]; then printf ' --disable-tls'; fi)"+
			" --cache-directory=%[7]s%[11]s"+
			" --bucket=\"$%[3]s\" --endpoint=\"$%[4]s\" --region=\"$%[9]s\""+
			" --prefix=\"$%[10]s\" --access-key=\"$%[5]s\" --secret-access-key=\"$%[6]s\"",
		Command(),
		action,
		BucketKey,
		EndpointKey,
		util.AWSAccessKey,
		util.AWSSecretKey,
		CacheDir,
		DisableTLSKey,
		RegionKey,
		PrefixKey,
		cacheFlags(),
	)
}

func cacheFlags() string {
	return fmt.Sprintf(
		" --content-cache-size-mb=\"$%s\" --content-cache-size-limit-mb=\"$%s\" --metadata-cache-size-mb=\"$%s\" --metadata-cache-size-limit-mb=\"$%s\"",
		ContentCacheSizeMBKey,
		ContentCacheSizeLimitMBKey,
		MetadataCacheSizeMBKey,
		MetadataCacheSizeLimitMBKey,
	)
}

func JobLabels(labels map[string]string) map[string]string {
	result := map[string]string{
		LabelKopiaJob: LabelValueTrue,
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
	repository *Repository,
	labels map[string]string,
) (*JobRuntime, bool, error) {
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
	cacheEnv, err := CacheConfigEnv()
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
		Env:       append(Env(secretName, repository), cacheEnv...),
		Resources: resources,
		Command:   Command(),
	}
	if cacheVolume != nil {
		runtime.Volumes = append(runtime.Volumes, *cacheVolume)
	}
	if cacheMount != nil {
		runtime.VolumeMounts = append(runtime.VolumeMounts, *cacheMount)
	}
	return runtime, true, nil
}

func CacheConfigEnv() ([]corev1.EnvVar, error) {
	sizeLimit, err := resource.ParseQuantity(settings.KopiaCacheSize.Get())
	if err != nil {
		return nil, fmt.Errorf("failed to parse setting %s=%q: %w",
			settings.KopiaCacheSizeSettingName, settings.KopiaCacheSize.Get(), err)
	}
	totalMiB := sizeLimit.Value() / bytesPerMiB
	if totalMiB < cacheHardLimitShareDenominator*cacheCapacityReservedFraction {
		return nil, fmt.Errorf("setting %s=%q must be at least 8Mi",
			settings.KopiaCacheSizeSettingName, settings.KopiaCacheSize.Get())
	}

	// Content and metadata each receive one quarter of the emptyDir as a hard
	// limit. The other half remains available for indexes, logs, and cache sweep
	// overlap. Soft limits are half of the corresponding hard limits.
	hardLimitMiB := totalMiB / cacheHardLimitShareDenominator
	softLimitMiB := hardLimitMiB / cacheCapacityReservedFraction
	return []corev1.EnvVar{
		{Name: ContentCacheSizeMBKey, Value: fmt.Sprintf("%d", softLimitMiB)},
		{Name: ContentCacheSizeLimitMBKey, Value: fmt.Sprintf("%d", hardLimitMiB)},
		{Name: MetadataCacheSizeMBKey, Value: fmt.Sprintf("%d", softLimitMiB)},
		{Name: MetadataCacheSizeLimitMBKey, Value: fmt.Sprintf("%d", hardLimitMiB)},
	}, nil
}

func (r *JobRuntime) VolumesWith(volumes ...corev1.Volume) []corev1.Volume {
	result := append([]corev1.Volume{}, r.Volumes...)
	return append(result, volumes...)
}

func CacheVolumeAndMount() (*corev1.Volume, *corev1.VolumeMount, error) {
	sizeLimit, err := resource.ParseQuantity(settings.KopiaCacheSize.Get())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse setting %s=%q: %w",
			settings.KopiaCacheSizeSettingName, settings.KopiaCacheSize.Get(), err)
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
	if err := addQuantity(resources.Requests, corev1.ResourceEphemeralStorage, settings.KopiaEphemeralRequest); err != nil {
		return corev1.ResourceRequirements{}, err
	}
	if err := addQuantity(resources.Limits, corev1.ResourceEphemeralStorage, settings.KopiaEphemeralLimit); err != nil {
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
	maxConcurrent := settings.KopiaMaxConcurrentJobs.GetInt()
	if maxConcurrent <= 0 {
		return true, nil
	}
	if clientset == nil {
		return false, fmt.Errorf("clientset is nil")
	}

	pods, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", LabelKopiaJob, LabelValueTrue),
	})
	if err != nil {
		return false, fmt.Errorf("listing kopia pods for scheduling: %w", err)
	}

	activeTotal := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		activeTotal++
	}
	return activeTotal < maxConcurrent, nil
}

func SnapshotTagsFlags(tags []string) string {
	flags := make([]string, 0, len(tags))
	for _, tag := range tags {
		flags = append(flags, "--tags "+shellQuote(tag))
	}
	return strings.Join(flags, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func SnapshotListCommand(tags []string) string {
	return fmt.Sprintf("%s snapshot list --all --json %s", Command(), SnapshotTagsFlags(tags))
}

func SnapshotCheckCommand(connectCommand string, tags []string) string {
	return fmt.Sprintf(
		"set -eo pipefail\n"+
			"%s\n"+
			"COUNT=$(%s | jq 'length')\n"+
			"test \"$COUNT\" != \"0\" || exit %d",
		connectCommand,
		SnapshotListCommand(tags),
		SnapshotCheckMissingExitCode,
	)
}

func SnapshotDeleteCommand(connectCommand string, tags []string) string {
	return fmt.Sprintf(
		"set -eo pipefail\n"+
			"%s\n"+
			"IDS=$(%s | jq -r '.[].id')\n"+
			"test -n \"$IDS\" || exit 0\n"+
			"%s snapshot delete --delete $IDS",
		connectCommand,
		SnapshotListCommand(tags),
		Command(),
	)
}

func SnapshotObjectCommand(connectCommand string, tags []string) string {
	return fmt.Sprintf(
		"%s\n"+
			"OBJ=$(%s | jq -er 'last | .rootEntry.obj') || { echo 'no matching Kopia snapshot found' >&2; exit 1; }\n"+
			"test -n \"$OBJ\" && test \"$OBJ\" != \"null\"",
		connectCommand,
		SnapshotListCommand(tags),
	)
}

func CheckSnapshotJob(ctx context.Context,
	clientset kubernetes.Interface,
	jobName string,
	getJob func(name string) (*batchv1.Job, error),
	createJob func(name string) error,
) (SnapshotCheckResult, error) {
	return snapshotcheck.CheckJob(ctx, clientset, snapshotcheck.CheckJobOptions{
		EngineName:      "kopia",
		ContainerName:   SnapshotCheckContainerName,
		MissingExitCode: SnapshotCheckMissingExitCode,
		JobName:         jobName,
		GetJob:          getJob,
		CreateJob:       createJob,
	})
}

func SnapshotCheckJobResult(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (SnapshotCheckResult, error) {
	return snapshotcheck.JobResult(ctx, clientset, job, "kopia", SnapshotCheckContainerName, SnapshotCheckMissingExitCode)
}

func SnapshotCheckJobExitCode(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (int32, bool, error) {
	return snapshotcheck.JobExitCode(ctx, clientset, job, "kopia", SnapshotCheckContainerName)
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
				ObjectMeta: metav1.ObjectMeta{Labels: opts.Labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         SnapshotCheckContainerName,
						Image:        opts.Runtime.Image,
						Env:          opts.Runtime.Env,
						Command:      []string{"/bin/sh", "-c"},
						Resources:    opts.Runtime.Resources,
						VolumeMounts: opts.Runtime.VolumeMounts,
						Args:         []string{SnapshotCheckCommand(opts.ConnectCommand, opts.Tags)},
					}},
					Volumes: opts.Runtime.Volumes,
				},
			},
		},
	}
}

func SnapshotTag(backupName string) string {
	return fmt.Sprintf("sn:%s", backupName)
}

func NamespaceTag(namespace string) string {
	return fmt.Sprintf("ns:%s", namespace)
}

func VMBackupTag(vmBackupName string) string {
	return fmt.Sprintf("vmb:%s", vmBackupName)
}

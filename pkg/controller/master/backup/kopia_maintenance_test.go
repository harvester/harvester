package backup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

func TestReconcileKopiaMaintenanceCreatesQuickAndFullCronJobs(t *testing.T) {
	prepareKopiaMaintenanceTest(t)
	clientset := fake.NewSimpleClientset()
	h := &TargetHandler{ctx: context.Background(), clientset: clientset}
	target := kopiaMaintenanceTarget(true)

	require.NoError(t, h.reconcileKopiaMaintenance(target))

	quick := getMaintenanceCronJob(t, clientset, kopiautil.QuickMaintenanceCronJobName)
	require.Equal(t, kopiautil.QuickMaintenanceSchedule, quick.Spec.Schedule)
	require.Equal(t, batchv1.ForbidConcurrent, quick.Spec.ConcurrencyPolicy)
	require.Contains(t, maintenanceCommand(quick), "maintenance set --owner=me")
	require.Contains(t, maintenanceCommand(quick), "maintenance run")
	require.NotContains(t, maintenanceCommand(quick), "maintenance run --full")
	require.Equal(t, "minio.example.com:9000", maintenanceEnvValue(quick, kopiautil.EndpointKey))
	require.Equal(t, kopiautil.PasswordKey, maintenanceSecretKey(quick, kopiautil.PasswordKey))

	full := getMaintenanceCronJob(t, clientset, kopiautil.FullMaintenanceCronJobName)
	require.Equal(t, kopiautil.FullMaintenanceSchedule, full.Spec.Schedule)
	require.Contains(t, maintenanceCommand(full), "maintenance set --owner=me")
	require.Contains(t, maintenanceCommand(full), "maintenance run --full")
}

func TestReconcileKopiaMaintenanceUpdatesChangedTarget(t *testing.T) {
	prepareKopiaMaintenanceTest(t)
	clientset := fake.NewSimpleClientset()
	h := &TargetHandler{ctx: context.Background(), clientset: clientset}
	target := kopiaMaintenanceTarget(true)
	require.NoError(t, h.reconcileKopiaMaintenance(target))

	target.Endpoint = "https://new.example.com"
	target.BucketRegion = "new-region"
	require.NoError(t, h.reconcileKopiaMaintenance(target))

	quick := getMaintenanceCronJob(t, clientset, kopiautil.QuickMaintenanceCronJobName)
	require.Equal(t, "new.example.com", maintenanceEnvValue(quick, kopiautil.EndpointKey))
	require.Equal(t, "new-region", maintenanceEnvValue(quick, kopiautil.RegionKey))
}

func TestReconcileKopiaMaintenanceRemovesCronJobsWhenDisabledOrTargetIsEmpty(t *testing.T) {
	prepareKopiaMaintenanceTest(t)

	for _, test := range []struct {
		name   string
		target *settings.BackupTarget
	}{
		{name: "disabled", target: kopiaMaintenanceTarget(false)},
		{name: "empty target", target: &settings.BackupTarget{}},
		{name: "deleted setting", target: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			h := &TargetHandler{ctx: context.Background(), clientset: clientset}
			require.NoError(t, h.reconcileKopiaMaintenance(kopiaMaintenanceTarget(true)))

			require.NoError(t, h.reconcileKopiaMaintenance(test.target))
			for _, name := range []string{kopiautil.QuickMaintenanceCronJobName, kopiautil.FullMaintenanceCronJobName} {
				_, err := clientset.BatchV1().CronJobs(util.LonghornSystemNamespaceName).
					Get(context.Background(), name, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "CronJob %s should be deleted, got %v", name, err)
			}
		})
	}
}

func TestDecodeBackupTargetKopiaGCEnabled(t *testing.T) {
	target, err := settings.DecodeBackupTarget(`{"type":"s3","kopiaGCEnabled":true}`)
	require.NoError(t, err)
	require.True(t, target.KopiaGCEnabled)
}

func TestRepositoryPasswordsAreDeterministic(t *testing.T) {
	target := kopiaMaintenanceTarget(false)
	data := map[string][]byte{util.AWSSecretKey: []byte("original-secret")}
	require.NoError(t, setRepositoryPasswords(data, target))
	require.NotEqual(t, data[util.AWSSecretKey], data[resticutil.PasswordKey])
	require.NotEqual(t, data[util.AWSSecretKey], data[kopiautil.PasswordKey])
	resticPassword := append([]byte(nil), data[resticutil.PasswordKey]...)
	kopiaPassword := append([]byte(nil), data[kopiautil.PasswordKey]...)

	data[util.AWSSecretKey] = []byte("rotated-secret")
	require.NoError(t, setRepositoryPasswords(data, target))
	require.Equal(t, resticPassword, data[resticutil.PasswordKey])
	require.Equal(t, kopiaPassword, data[kopiautil.PasswordKey])

	target.BucketName = "other"
	require.NoError(t, setRepositoryPasswords(data, target))
	require.NotEqual(t, resticPassword, data[resticutil.PasswordKey])
	require.NotEqual(t, kopiaPassword, data[kopiautil.PasswordKey])
}

func prepareKopiaMaintenanceTest(t *testing.T) {
	t.Helper()
	t.Setenv(kopiautil.ImageEnvVar, "example.com/harvester:test")

	cacheSize := settings.KopiaCacheSize.Get()
	ephemeralRequest := settings.KopiaEphemeralRequest.Get()
	ephemeralLimit := settings.KopiaEphemeralLimit.Get()
	require.NoError(t, settings.KopiaCacheSize.Set("2Gi"))
	require.NoError(t, settings.KopiaEphemeralRequest.Set("512Mi"))
	require.NoError(t, settings.KopiaEphemeralLimit.Set("3Gi"))
	t.Cleanup(func() {
		_ = settings.KopiaCacheSize.Set(cacheSize)
		_ = settings.KopiaEphemeralRequest.Set(ephemeralRequest)
		_ = settings.KopiaEphemeralLimit.Set(ephemeralLimit)
	})
}

func kopiaMaintenanceTarget(enabled bool) *settings.BackupTarget {
	return &settings.BackupTarget{
		Type:           settings.S3BackupType,
		Endpoint:       "http://minio.example.com:9000",
		BucketName:     "backups",
		BucketRegion:   "test-region",
		KopiaGCEnabled: enabled,
	}
}

func getMaintenanceCronJob(t *testing.T, clientset *fake.Clientset, name string) *batchv1.CronJob {
	t.Helper()
	cronJob, err := clientset.BatchV1().CronJobs(util.LonghornSystemNamespaceName).
		Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	return cronJob
}

func maintenanceContainer(cronJob *batchv1.CronJob) corev1.Container {
	return cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
}

func maintenanceCommand(cronJob *batchv1.CronJob) string {
	return maintenanceContainer(cronJob).Args[0]
}

func maintenanceEnvValue(cronJob *batchv1.CronJob, name string) string {
	for _, env := range maintenanceContainer(cronJob).Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

func maintenanceSecretKey(cronJob *batchv1.CronJob, name string) string {
	for _, env := range maintenanceContainer(cronJob).Env {
		if env.Name == name && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			return env.ValueFrom.SecretKeyRef.Key
		}
	}
	return ""
}

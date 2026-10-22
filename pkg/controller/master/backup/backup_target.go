package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

const (
	backupTargetControllerName = "harvester-backup-target-controller"
)

// RegisterBackupTarget register the setting controller and reconsile longhorn setting when backup target changed
func RegisterBackupTarget(ctx context.Context, management *config.Management, _ config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	lhBackupTargets := management.LonghornFactory.Longhorn().V1beta2().BackupTarget()

	backupTargetController := &TargetHandler{
		ctx:                 ctx,
		secrets:             secrets,
		secretCache:         secrets.Cache(),
		settings:            settings,
		lhBackupTargets:     lhBackupTargets,
		lhBackupTargetCache: lhBackupTargets.Cache(),
		clientset:           management.ClientSet,
	}

	settings.OnChange(ctx, backupTargetControllerName, backupTargetController.OnBackupTargetChange)
	return nil
}

type TargetHandler struct {
	ctx                 context.Context
	secrets             ctlcorev1.SecretClient
	secretCache         ctlcorev1.SecretCache
	settings            ctlharvesterv1.SettingClient
	lhBackupTargets     ctllonghornv1.BackupTargetClient
	lhBackupTargetCache ctllonghornv1.BackupTargetCache
	clientset           kubernetes.Interface
}

// OnBackupTargetChange handles backupTarget setting object on change
func (h *TargetHandler) OnBackupTargetChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.Name != settings.BackupTargetSettingName {
		return setting, nil
	}
	if setting.DeletionTimestamp != nil {
		return setting, h.reconcileKopiaMaintenance(nil)
	}

	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return h.setConfiguredCondition(setting, "", err)
	}

	logrus.Debugf("backup target change:%s:%s", target.Type, target.Endpoint)

	switch target.Type {
	case settings.S3BackupType:
		// S3 credentials are removed from setting.Value after being stored in the
		// backup-target Secret. On the resulting reconciliation, only refresh the
		// deterministic repository passwords.
		if target.SecretAccessKey == "" && target.AccessKeyID == "" {
			if err = h.syncRepositoryPasswords(target); err != nil {
				return h.setConfiguredCondition(setting, "", err)
			}
			break
		}
		// A submitted target with credentials represents a newly configured or
		// changed S3 target. Stop the old repository maintenance schedules before
		// replacing the shared credentials and repository parameters.
		if err = h.reconcileKopiaMaintenance(nil); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

		if err = h.updateLonghornTarget(target); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

		if err = h.syncBackupSecret(target); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

		if setting, err = h.stripBackupTargetCredentials(setting, target); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

	case settings.NFSBackupType:
		if err = h.updateLonghornTarget(target); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

		// reset previous secret of S3
		if err = h.resetBackupTargetSecret(); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}

	default:
		// reset backup target to default, then delete/update related settings
		if target.IsDefaultBackupTarget() {
			if err = h.reconcileKopiaMaintenance(nil); err != nil {
				return h.setConfiguredCondition(setting, "", err)
			}
			if err = h.updateLonghornTarget(target); err != nil {
				return h.setConfiguredCondition(setting, "", err)
			}

			// reset previous secret of S3
			if err = h.resetBackupTargetSecret(); err != nil {
				return h.setConfiguredCondition(setting, "", err)
			}

			settingCpy := setting.DeepCopy()
			harvesterv1.SettingConfigured.False(settingCpy)
			harvesterv1.SettingConfigured.Message(settingCpy, "")
			harvesterv1.SettingConfigured.Reason(settingCpy, "")
			return h.settings.Update(settingCpy)
		}

		return h.setConfiguredCondition(setting, "", fmt.Errorf("invalid backup target type:%s or parameter", target.Type))
	}

	if target.RefreshIntervalInSeconds > 0 {
		if err = h.updateLHBackupTargetPollInterval(target); err != nil {
			return h.setConfiguredCondition(setting, "", err)
		}
	}

	if err = h.reconcileKopiaMaintenance(target); err != nil {
		return h.setConfiguredCondition(setting, "", err)
	}

	if len(setting.Status.Conditions) == 0 || harvesterv1.SettingConfigured.IsFalse(setting) {
		return h.setConfiguredCondition(setting, "", nil)
	}
	return setting, nil
}

type kopiaMaintenanceSchedule struct {
	name     string
	schedule string
	full     bool
}

var kopiaMaintenanceSchedules = []kopiaMaintenanceSchedule{
	{name: kopiautil.QuickMaintenanceCronJobName, schedule: kopiautil.QuickMaintenanceSchedule},
	{name: kopiautil.FullMaintenanceCronJobName, schedule: kopiautil.FullMaintenanceSchedule, full: true},
}

// reconcileKopiaMaintenance owns the cluster-wide Kopia maintenance schedules.
// A nil, default, non-S3, or disabled target removes both CronJobs. Otherwise it
// creates or updates the schedules to match the current backup-target setting.
func (h *TargetHandler) reconcileKopiaMaintenance(target *settings.BackupTarget) error {
	if h.clientset == nil {
		return fmt.Errorf("kubernetes clientset is nil")
	}

	cronJobs := h.clientset.BatchV1().CronJobs(util.LonghornSystemNamespaceName)
	if target == nil || target.IsDefaultBackupTarget() || target.Type != settings.S3BackupType || !target.KopiaGCEnabled {
		for _, maintenance := range kopiaMaintenanceSchedules {
			if err := cronJobs.Delete(h.ctx, maintenance.name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete Kopia maintenance CronJob %s/%s: %w",
					util.LonghornSystemNamespaceName, maintenance.name, err)
			}
		}
		return nil
	}

	repository, err := kopiautil.S3Repository(target)
	if err != nil {
		return err
	}
	for _, maintenance := range kopiaMaintenanceSchedules {
		desired, err := kopiautil.NewMaintenanceCronJob(
			maintenance.name,
			maintenance.schedule,
			maintenance.full,
			util.BackupTargetSecretName,
			repository,
		)
		if err != nil {
			return fmt.Errorf("failed to build Kopia maintenance CronJob %s: %w", maintenance.name, err)
		}

		current, err := cronJobs.Get(h.ctx, maintenance.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err = cronJobs.Create(h.ctx, desired, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create Kopia maintenance CronJob %s/%s: %w",
					desired.Namespace, desired.Name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get Kopia maintenance CronJob %s/%s: %w",
				desired.Namespace, desired.Name, err)
		}
		if apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
			apiequality.Semantic.DeepDerivative(desired.Spec, current.Spec) {
			continue
		}

		desired.ResourceVersion = current.ResourceVersion
		if _, err = cronJobs.Update(h.ctx, desired, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update Kopia maintenance CronJob %s/%s: %w",
				desired.Namespace, desired.Name, err)
		}
	}
	return nil
}

func (h *TargetHandler) stripBackupTargetCredentials(
	setting *harvesterv1.Setting,
	target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	// only do a second update when s3 with credentials
	if target.Type != settings.S3BackupType {
		return nil, nil
	}

	// Remove S3 credentials from setting.Value after they have been stored in
	// the backup-target Secret. The resulting reconciliation skips credential
	// synchronization because both credentials are empty.
	target.SecretAccessKey = ""
	target.AccessKeyID = ""
	targetBytes, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}

	settingCpy := setting.DeepCopy()
	settingCpy.Value = string(targetBytes)

	return h.settings.Update(settingCpy)
}

func (h *TargetHandler) updateLonghornTarget(backupTarget *settings.BackupTarget) error {
	lhBackupTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
	if err != nil {
		return err
	}

	lhBackupTargetCpy := lhBackupTarget.DeepCopy()
	lhBackupTargetCpy.Spec.BackupTargetURL = backuputil.ConstructEndpoint(backupTarget)

	if reflect.DeepEqual(lhBackupTarget, lhBackupTargetCpy) {
		return nil
	}

	_, err = h.lhBackupTargets.Update(lhBackupTargetCpy)
	return err
}

func getBackupSecretData(target *settings.BackupTarget) (map[string]string, error) {
	data := map[string]string{
		util.AWSAccessKey:       target.AccessKeyID,
		util.AWSSecretKey:       target.SecretAccessKey,
		util.AWSEndpoints:       target.Endpoint,
		util.AWSCERT:            target.Cert,
		util.VirtualHostedStyle: strconv.FormatBool(target.VirtualHostedStyle),
	}
	if settings.AdditionalCA.Get() != "" {
		data[util.AWSCERT] = settings.AdditionalCA.Get()
	}

	var httpProxyConfig util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(settings.HTTPProxy.Get()), &httpProxyConfig); err != nil {
		return nil, err
	}
	data[util.HTTPProxyEnv] = httpProxyConfig.HTTPProxy
	data[util.HTTPSProxyEnv] = httpProxyConfig.HTTPSProxy
	data[util.NoProxyEnv] = util.AddBuiltInNoProxy(httpProxyConfig.NoProxy)

	// trim spaces for all values in order to pass the Longhorn webhook, refer to https://github.com/longhorn/longhorn-manager/pull/970
	for k, v := range data {
		data[k] = strings.TrimSpace(v)
	}

	return data, nil
}

func (h *TargetHandler) syncBackupSecret(target *settings.BackupTarget) error {
	values, err := getBackupSecretData(target)
	if err != nil {
		return err
	}
	data := stringMapToBytes(values)
	if err := setRepositoryPasswords(data, target); err != nil {
		return err
	}

	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	switch {
	case apierrors.IsNotFound(err):
		err = h.createBackupSecret(data)
	case err != nil:
		return err
	default:
		err = h.updateBackupSecret(secret, data)
	}
	if err != nil {
		return err
	}

	return h.updateLonghornBackupTargetSecretSetting()
}

func (h *TargetHandler) createBackupSecret(data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.BackupTargetSecretName,
			Namespace: util.LonghornSystemNamespaceName,
		},
		Data: data,
	}
	_, err := h.secrets.Create(secret)
	return err
}

func (h *TargetHandler) updateBackupSecret(secret *corev1.Secret, data map[string][]byte) error {
	secretCpy := secret.DeepCopy()
	secretCpy.Data = data
	secretCpy.StringData = nil
	if reflect.DeepEqual(secret.Data, secretCpy.Data) {
		return nil
	}
	_, err := h.secrets.Update(secretCpy)
	return err
}

func (h *TargetHandler) resetBackupTargetSecret() error {
	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if secret != nil && len(secret.Data) != 0 {
		secretCpy := secret.DeepCopy()
		secretCpy.Data = map[string][]byte{}
		secretCpy.StringData = nil
		if !reflect.DeepEqual(secret.Data, secretCpy.Data) {
			_, err = h.secrets.Update(secretCpy)
		}
		if err != nil {
			return err
		}
	}

	lhBackupTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
	if err != nil {
		return err
	}

	lhBackupTargetCpy := lhBackupTarget.DeepCopy()
	lhBackupTargetCpy.Spec.CredentialSecret = ""

	if reflect.DeepEqual(lhBackupTarget, lhBackupTargetCpy) {
		return nil
	}

	_, err = h.lhBackupTargets.Update(lhBackupTargetCpy)
	return err
}

func (h *TargetHandler) syncRepositoryPasswords(target *settings.BackupTarget) error {
	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		return fmt.Errorf("failed to get backup target secret: %w", err)
	}
	data := secret.DeepCopy().Data
	if data == nil {
		data = map[string][]byte{}
	}
	if err := setRepositoryPasswords(data, target); err != nil {
		return err
	}
	return h.updateBackupSecret(secret, data)
}

func setRepositoryPasswords(data map[string][]byte, target *settings.BackupTarget) error {
	repositoryID := backuputil.RepositoryID(target)
	for backupType, key := range map[harvesterv1.BackupType]string{
		harvesterv1.Restic: resticutil.PasswordKey,
		harvesterv1.Kopia:  kopiautil.PasswordKey,
	} {
		password, err := backuputil.RepositoryPassword(repositoryID, backupType)
		if err != nil {
			return err
		}
		data[key] = []byte(password)
	}
	return nil
}

func stringMapToBytes(data map[string]string) map[string][]byte {
	result := make(map[string][]byte, len(data))
	for key, value := range data {
		result[key] = []byte(value)
	}
	return result
}

func (h *TargetHandler) updateLonghornBackupTargetSecretSetting() error {
	lhBackupTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
	if err != nil {
		return err
	}

	lhBackupTargetCpy := lhBackupTarget.DeepCopy()
	lhBackupTargetCpy.Spec.CredentialSecret = util.BackupTargetSecretName

	if reflect.DeepEqual(lhBackupTarget, lhBackupTargetCpy) {
		return nil
	}

	_, err = h.lhBackupTargets.Update(lhBackupTargetCpy)
	return err
}

func (h *TargetHandler) setConfiguredCondition(setting *harvesterv1.Setting, reason string, err error) (*harvesterv1.Setting, error) {
	settingCpy := setting.DeepCopy()
	// SetError with nil error will cleanup message in condition and set the status to true
	harvesterv1.SettingConfigured.SetError(settingCpy, reason, err)
	return h.settings.Update(settingCpy)
}

func (h *TargetHandler) updateLHBackupTargetPollInterval(backupTarget *settings.BackupTarget) error {
	lhBackupTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
	if err != nil {
		return err
	}

	lhBackupTargetCpy := lhBackupTarget.DeepCopy()
	lhBackupTargetCpy.Spec.PollInterval = metav1.Duration{
		Duration: time.Duration(backupTarget.RefreshIntervalInSeconds) * time.Second,
	}

	if reflect.DeepEqual(lhBackupTarget, lhBackupTargetCpy) {
		return nil
	}

	_, err = h.lhBackupTargets.Update(lhBackupTargetCpy)
	return err
}

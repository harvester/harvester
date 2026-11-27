package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"time"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
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
	"github.com/harvester/harvester/pkg/util/repository"
)

const (
	backupTargetControllerName = "harvester-backup-target-controller"
)

// RegisterBackupTarget register the setting controller and reconsile longhorn setting when backup target changed
func RegisterBackupTarget(ctx context.Context, management *config.Management, _ config.Options) error {
	settingController := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	lhBackupTargets := management.LonghornFactory.Longhorn().V1beta2().BackupTarget()
	repositories := &repository.Manager{}

	h := &TargetHandler{
		ctx:                 ctx,
		secrets:             secrets,
		settings:            settingController,
		lhBackupTargets:     lhBackupTargets,
		lhBackupTargetCache: lhBackupTargets.Cache(),
		clientset:           management.ClientSet,
		initRepos:           repositories.Init,
		connectRepos:        repositories.Connect,
	}

	settingController.OnChange(ctx, backupTargetControllerName, h.OnBackupTargetChange)
	settingController.OnChange(ctx, backupTargetControllerName+"-network",
		func(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
			if key == settings.AdditionalCASettingName || key == settings.HTTPProxySettingName {
				settingController.Enqueue(settings.BackupTargetSettingName)
			}
			return setting, nil
		})
	secrets.OnChange(ctx, backupTargetControllerName,
		func(key string, secret *corev1.Secret) (*corev1.Secret, error) {
			if key == util.LonghornSystemNamespaceName+"/"+util.BackupTargetSecretName {
				settingController.Enqueue(settings.BackupTargetSettingName)
			}
			return secret, nil
		})
	return nil
}

type TargetHandler struct {
	ctx                 context.Context
	secrets             ctlcorev1.SecretClient
	settings            ctlharvesterv1.SettingClient
	lhBackupTargets     ctllonghornv1.BackupTargetClient
	lhBackupTargetCache ctllonghornv1.BackupTargetCache
	clientset           kubernetes.Interface
	initRepos           func(context.Context, *settings.BackupTarget, map[string][]byte) error
	connectRepos        func(context.Context, *settings.BackupTarget, map[string][]byte) error
}

// OnBackupTargetChange dispatches backup-related Setting changes.
func (h *TargetHandler) OnBackupTargetChange(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil {
		return setting, nil
	}
	switch setting.Name {
	case settings.ThirdPartyBackupConfigSettingName:
		return h.reconcileThirdPartyConfig(setting)
	case settings.BackupTargetSettingName:
		return h.reconcileTarget(setting)
	default:
		return setting, nil
	}
}

func (h *TargetHandler) reconcileTarget(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting.DeletionTimestamp != nil {
		return setting, h.reconcileKopiaMaintenance(nil)
	}
	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return h.reportTargetError(setting, err)
	}
	logrus.Debugf("backup target change:%s:%s", target.Type, target.Endpoint)

	setting, err = h.syncTarget(setting, target)
	if err != nil {
		return h.reportTargetError(setting, err)
	}
	setting, err = h.updateConfiguredStatus(setting, target)
	if err != nil {
		return setting, err
	}
	return setting, h.reconcileKopiaMaintenance(target)
}

func (h *TargetHandler) reportTargetError(
	setting *harvesterv1.Setting, err error,
) (*harvesterv1.Setting, error) {
	if apierrors.IsConflict(err) {
		return setting, err
	}
	return h.setConfiguredCondition(setting, "", err)
}

func (h *TargetHandler) updateConfiguredStatus(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	if err := h.checkThirdPartyReceipt(setting, target); err != nil {
		return setting, err
	}
	if target.IsDefaultBackupTarget() {
		return h.markUnconfigured(setting)
	}
	if len(setting.Status.Conditions) == 0 || harvesterv1.SettingConfigured.IsFalse(setting) {
		return h.setConfiguredCondition(setting, "", nil)
	}
	return setting, nil
}

func (h *TargetHandler) checkThirdPartyReceipt(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) error {
	if !target.ThirdPartyBackupEnabled {
		return nil
	}
	secret, err := h.getBackupSecret()
	if err != nil {
		return err
	}
	return backuputil.CheckRepoReceipt(setting, target, secret)
}

func (h *TargetHandler) markUnconfigured(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	updated := setting.DeepCopy()
	harvesterv1.SettingConfigured.False(updated)
	harvesterv1.SettingConfigured.Message(updated, "")
	harvesterv1.SettingConfigured.Reason(updated, "")
	return h.settings.Update(updated)
}

func (h *TargetHandler) syncTarget(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	setting, err := h.clearDisabledReceipt(setting, target)
	if err != nil {
		return setting, err
	}
	if target.IsDefaultBackupTarget() {
		return setting, h.resetTarget(target)
	}

	setting, err = h.syncRemoteTarget(setting, target)
	if err != nil {
		return setting, err
	}
	return setting, h.updateLHBackupTargetPollInterval(target)
}

func (h *TargetHandler) syncRemoteTarget(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	switch target.Type {
	case settings.S3BackupType:
		return h.syncS3Target(setting, target)
	case settings.NFSBackupType:
		return setting, h.syncNFSTarget(target)
	default:
		return setting, fmt.Errorf("invalid backup target type:%s or parameter", target.Type)
	}
}

func (h *TargetHandler) syncS3Target(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	if err := h.checkThirdPartyOptions(target); err != nil {
		return setting, err
	}
	if backuputil.Masked(target) {
		return h.reconcileMaskedS3(setting, target)
	}
	if target.ThirdPartyBackupEnabled {
		return h.prepareTarget(setting, target)
	}
	return h.syncLonghornOnly(setting, target)
}

// checkThirdPartyOptions stops maintenance when an enabled target uses
// unsupported options, retaining both validation and cleanup errors.
func (h *TargetHandler) checkThirdPartyOptions(target *settings.BackupTarget) error {
	if !target.ThirdPartyBackupEnabled {
		return nil
	}
	client := backuputil.TargetClient{Settings: h.settings}
	if err := client.CheckThirdPartyOptions(target); err != nil {
		return errors.Join(err, h.stopMaintenance())
	}
	return nil
}

func (h *TargetHandler) reconcileMaskedS3(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	secret, err := h.checkSavedSecret(target)
	if err != nil {
		return setting, err
	}
	if !target.ThirdPartyBackupEnabled {
		return setting, nil
	}
	// A valid receipt also covers an interrupted configured status update.
	if backuputil.CheckRepoReceipt(setting, target, secret) == nil {
		return setting, nil
	}
	return h.refreshRepoReceipt(setting, target, secret)
}

// syncLonghornOnly applies an unmasked S3 submission with third-party backups disabled.
func (h *TargetHandler) syncLonghornOnly(
	setting *harvesterv1.Setting, target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	if err := h.saveS3Only(target); err != nil {
		return setting, err
	}
	if err := h.syncLonghornS3(target); err != nil {
		return setting, err
	}
	return h.stripCredentials(setting, target)
}

func (h *TargetHandler) syncNFSTarget(target *settings.BackupTarget) error {
	if err := h.stopMaintenance(); err != nil {
		return err
	}
	if err := h.updateLonghornTarget(target); err != nil {
		return err
	}
	return h.resetBackupTargetSecret()
}

// checkSavedSecret only validates persisted credentials after masking.
// Longhorn needs AWS credentials even when third-party backups are disabled.
func (h *TargetHandler) checkSavedSecret(target *settings.BackupTarget) (*corev1.Secret, error) {
	secret, err := h.getBackupSecret()
	if err != nil {
		return nil, err
	}
	data := secretData(secret)
	if err := checkS3Credentials(data); err != nil {
		return nil, err
	}
	if target.ThirdPartyBackupEnabled {
		if err := checkRepoCredentials(data, backuputil.RepositoryID(target)); err != nil {
			return nil, err
		}
	}
	return secret, nil
}

func (h *TargetHandler) resetTarget(target *settings.BackupTarget) error {
	if err := h.stopMaintenance(); err != nil {
		return err
	}
	if err := h.updateLonghornTarget(target); err != nil {
		return err
	}
	return h.resetBackupTargetSecret()
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

	if target == nil || target.IsDefaultBackupTarget() || target.Type != settings.S3BackupType {
		return h.deleteKopiaMaintenance()
	}
	if !target.ThirdPartyBackupEnabled || !target.KopiaGCEnabled {
		return h.deleteKopiaMaintenance()
	}
	client := backuputil.TargetClient{Settings: h.settings, Secrets: h.secrets}
	setting, current, err := client.Get()
	if err != nil {
		return err
	}
	if err := client.CheckThirdPartyOptions(current); err != nil {
		return h.stopMaintenance()
	}
	if _, err := client.ReadyRepoSecret(setting, current); err != nil {
		return h.deleteKopiaMaintenance()
	}
	return h.syncMaintenanceSchedules(current)
}

func (h *TargetHandler) syncMaintenanceSchedules(target *settings.BackupTarget) error {
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

		if err := h.syncMaintenanceCron(desired); err != nil {
			return err
		}
	}
	return nil
}

func (h *TargetHandler) deleteKopiaMaintenance() error {
	for _, maintenance := range kopiaMaintenanceSchedules {
		if err := h.deleteMaintenanceCron(maintenance.name); err != nil {
			return err
		}
	}
	return nil
}

func (h *TargetHandler) deleteMaintenanceCron(name string) error {
	cronJobs := h.clientset.BatchV1().CronJobs(util.LonghornSystemNamespaceName)
	err := cronJobs.Delete(h.ctx, name, metav1.DeleteOptions{
		PropagationPolicy: new(metav1.DeletePropagationForeground),
	})
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf("failed to delete Kopia maintenance CronJob %s/%s: %w",
		util.LonghornSystemNamespaceName, name, err)
}

func (h *TargetHandler) syncMaintenanceCron(desired *batchv1.CronJob) error {
	cronJobs := h.clientset.BatchV1().CronJobs(desired.Namespace)
	current, err := cronJobs.Get(h.ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return h.createMaintenanceCron(desired)
	}
	if err != nil {
		return fmt.Errorf("failed to get Kopia maintenance CronJob %s/%s: %w",
			desired.Namespace, desired.Name, err)
	}
	if maintenanceMatches(current, desired) {
		return nil
	}
	return h.updateMaintenanceCron(current, desired)
}

func (h *TargetHandler) updateMaintenanceCron(current, desired *batchv1.CronJob) error {
	cronJobs := h.clientset.BatchV1().CronJobs(desired.Namespace)
	desired.ResourceVersion = current.ResourceVersion
	if _, err := cronJobs.Update(h.ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update Kopia maintenance CronJob %s/%s: %w",
			desired.Namespace, desired.Name, err)
	}
	return nil
}

func (h *TargetHandler) createMaintenanceCron(desired *batchv1.CronJob) error {
	cronJobs := h.clientset.BatchV1().CronJobs(desired.Namespace)
	_, err := cronJobs.Create(h.ctx, desired, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Kopia maintenance CronJob %s/%s: %w",
			desired.Namespace, desired.Name, err)
	}
	return nil
}

func maintenanceMatches(current, desired *batchv1.CronJob) bool {
	containers := current.Spec.JobTemplate.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		return false
	}
	// DeepDerivative ignores removed map entries. Compare resources explicitly
	// so resource overrides absent from the desired template are removed.
	desiredResources := desired.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources
	return apiequality.Semantic.DeepEqual(containers[0].Resources, desiredResources) &&
		apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepDerivative(desired.Spec, current.Spec)
}

func (h *TargetHandler) stripCredentials(
	setting *harvesterv1.Setting,
	target *settings.BackupTarget,
) (*harvesterv1.Setting, error) {
	if target.AccessKeyID == "" && target.SecretAccessKey == "" && target.RepoPassword == "" {
		return setting, nil
	}

	// Clear submitted credentials only after they have been saved to the Secret.
	target.SecretAccessKey = ""
	target.AccessKeyID = ""
	target.RepoPassword = ""
	targetBytes, err := json.Marshal(target)
	if err != nil {
		return setting, err
	}

	settingCpy := setting.DeepCopy()
	settingCpy.Value = string(targetBytes)
	updated, err := h.settings.Update(settingCpy)
	if err != nil {
		return setting, err
	}
	return updated, nil
}

func (h *TargetHandler) syncLonghornS3(target *settings.BackupTarget) error {
	if err := h.updateLonghornTarget(target); err != nil {
		return err
	}
	return h.setLonghornSecret(util.BackupTargetSecretName)
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

// saveS3Only stores submitted S3 credentials and the repository ID, clearing
// Restic and Kopia passwords because third-party backups are disabled.
func (h *TargetHandler) saveS3Only(target *settings.BackupTarget) error {
	if target.RepoPassword != "" {
		return fmt.Errorf("thirdPartyRepositoryPassword requires third-party backups to be enabled")
	}
	secret, data, err := h.prepareS3Secret(target)
	if err != nil {
		return err
	}
	delete(data, backuputil.ResticPasswordKey)
	delete(data, backuputil.KopiaPasswordKey)
	return h.saveS3Secret(secret, data, target)
}

func checkRepoCredentials(data map[string][]byte, repositoryID string) error {
	if string(data[backuputil.RepositoryIDKey]) != repositoryID {
		return fmt.Errorf("backup target Secret repository ID does not match the configured backup target")
	}
	return backuputil.CheckRepositoryPasswords(data)
}

// prepareS3Secret copies submitted S3 fields without changing repository fields
// or writing any resources. Both user-submission paths use this preparation.
func (h *TargetHandler) prepareS3Secret(
	target *settings.BackupTarget,
) (*corev1.Secret, map[string][]byte, error) {
	if target.AccessKeyID == "" || target.SecretAccessKey == "" {
		return nil, nil, fmt.Errorf("S3 backup target requires both AWS access key and secret key")
	}
	secret, err := h.getBackupSecret()
	if err != nil {
		return nil, nil, err
	}
	values, err := getBackupSecretData(target)
	if err != nil {
		return nil, nil, err
	}
	data := secretData(secret)
	maps.Copy(data, stringMapToBytes(values))
	if err := checkS3Credentials(data); err != nil {
		return nil, nil, err
	}
	return secret, data, nil
}

// saveS3Secret stops maintenance before storing validated credentials and their
// repository ID. The caller has already set or cleared repository passwords.
func (h *TargetHandler) saveS3Secret(
	secret *corev1.Secret, data map[string][]byte, target *settings.BackupTarget,
) error {
	// Stop old maintenance before replacing shared credentials and target parameters.
	if err := h.stopMaintenance(); err != nil {
		return err
	}
	data[backuputil.RepositoryIDKey] = []byte(backuputil.RepositoryID(target))
	return h.saveBackupSecret(secret, data)
}

func (h *TargetHandler) getBackupSecret() (*corev1.Secret, error) {
	secret, err := h.secrets.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return secret, err
}

func secretData(secret *corev1.Secret) map[string][]byte {
	if secret == nil || secret.Data == nil {
		return map[string][]byte{}
	}
	return secret.DeepCopy().Data
}

func (h *TargetHandler) saveBackupSecret(secret *corev1.Secret, data map[string][]byte) error {
	if secret == nil {
		return h.createBackupSecret(data)
	}
	return h.updateBackupSecret(secret, data)
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
	if err := h.clearS3Credentials(); err != nil {
		return err
	}
	return h.setLonghornSecret("")
}

// clearS3Credentials removes S3 and repository fields, preserving unrelated Secret data.
func (h *TargetHandler) clearS3Credentials() error {
	secret, err := h.getBackupSecret()
	if err != nil || secret == nil {
		return err
	}
	data := secretData(secret)
	// A default or NFS target clears all S3 and repository credentials.
	for _, key := range []string{
		util.AWSAccessKey, util.AWSSecretKey, util.AWSEndpoints, util.AWSCERT,
		util.VirtualHostedStyle, util.HTTPProxyEnv, util.HTTPSProxyEnv, util.NoProxyEnv,
		backuputil.RepositoryIDKey, backuputil.ResticPasswordKey, backuputil.KopiaPasswordKey,
	} {
		delete(data, key)
	}
	return h.updateBackupSecret(secret, data)
}

func checkS3Credentials(data map[string][]byte) error {
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey} {
		if len(data[key]) == 0 {
			return fmt.Errorf("backup target Secret is missing %s", key)
		}
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

func (h *TargetHandler) setLonghornSecret(name string) error {
	lhBackupTarget, err := h.lhBackupTargetCache.Get(util.LonghornSystemNamespaceName, longhorntypes.DefaultBackupTargetName)
	if err != nil {
		return err
	}

	lhBackupTargetCpy := lhBackupTarget.DeepCopy()
	lhBackupTargetCpy.Spec.CredentialSecret = name

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
	if reflect.DeepEqual(setting, settingCpy) {
		return setting, err
	}
	return h.settings.Update(settingCpy)
}

func (h *TargetHandler) updateLHBackupTargetPollInterval(backupTarget *settings.BackupTarget) error {
	if backupTarget.RefreshIntervalInSeconds <= 0 {
		return nil
	}
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

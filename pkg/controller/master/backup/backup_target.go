package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	backupTargetControllerName = "harvester-backup-target-controller"

	longhornBackupTargetSettingName       = "backup-target"
	longhornBackupTargetSecretSettingName = "backup-target-credential-secret"

	AWSAccessKey       = "AWS_ACCESS_KEY_ID"
	AWSSecretKey       = "AWS_SECRET_ACCESS_KEY"
	AWSEndpoints       = "AWS_ENDPOINTS"
	AWSCERT            = "AWS_CERT"
	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"
)

// RegisterBackupTarget register the setting controller and reconsile longhorn setting when backup target server
func RegisterBackupTarget(ctx context.Context, management *config.Management, opts config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta1().Setting()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()

	backupTargetController := &TargetHandler{
		ctx:                  ctx,
		longhornSettings:     longhornSettings,
		longhornSettingCache: longhornSettings.Cache(),
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		vms:                  vms,
		settings:             settings,
	}

	settings.OnChange(ctx, backupTargetControllerName, backupTargetController.OnBackupTargetChange)
	return nil
}

type TargetHandler struct {
	ctx                  context.Context
	longhornSettings     ctllonghornv1.SettingClient
	longhornSettingCache ctllonghornv1.SettingCache
	secrets              ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	vms                  ctlkubevirtv1.VirtualMachineController
	settings             ctlharvesterv1.SettingClient
}

// OnBackupTargetChange handles backupTarget setting object on change
func (h *TargetHandler) OnBackupTargetChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil ||
		setting.Name != settings.BackupTargetSettingName || setting.Value == "" {
		return nil, nil
	}

	target, err := settings.DecodeBackupTarget(setting.Value)
	if err != nil {
		return setting, err
	}

	// Since we remove S3 access key id and secret access key after S3 backup target has been verified,
	// we stop the controller to reconcile it again.
	if target.Type == settings.S3BackupType && (target.SecretAccessKey == "" || target.AccessKeyID == "") {
		return nil, nil
	}

	if err = h.updateLonghornTarget(target); err != nil {
		return nil, err
	}

	if target.Type == settings.S3BackupType {
		if err = h.updateBackupTargetSecret(target); err != nil {
			return nil, err
		}
	}

	return h.updateBackupTargetSetting(setting.DeepCopy(), target, err)
}

func (h *TargetHandler) updateBackupTargetSetting(setting *harvesterv1.Setting, target *settings.BackupTarget, err error) (*harvesterv1.Setting, error) {
	harvesterv1.SettingConfigured.SetError(setting, "", err)

	// reset the s3 credentials to prevent controller reconcile and not to expose secret key
	target.SecretAccessKey = ""
	target.AccessKeyID = ""
	target.Endpoint = ConstructEndpoint(target)
	targetBytes, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}

	setting.Value = string(targetBytes)

	return h.settings.Update(setting)
}

func (h *TargetHandler) updateLonghornTarget(backupTarget *settings.BackupTarget) error {
	target, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornBackupTargetSettingName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		if _, err := h.longhornSettings.Create(&longhornv1.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      longhornBackupTargetSettingName,
				Namespace: util.LonghornSystemNamespaceName,
			},
			Setting: types.Setting{Value: ConstructEndpoint(backupTarget)},
		}); err != nil {
			return err
		}
		return nil
	}

	targetCpy := target.DeepCopy()
	targetCpy.Value = ConstructEndpoint(backupTarget)

	if !reflect.DeepEqual(target, targetCpy) {
		_, err := h.longhornSettings.Update(targetCpy)
		return err
	}
	return nil
}

func setBackupSecret(target *settings.BackupTarget) map[string]string {
	return map[string]string{
		AWSAccessKey:       target.AccessKeyID,
		AWSSecretKey:       target.SecretAccessKey,
		AWSCERT:            target.Cert,
		VirtualHostedStyle: strconv.FormatBool(target.VirtualHostedStyle),
	}
}

func (h *TargetHandler) updateBackupTargetSecret(target *settings.BackupTarget) error {
	var found = true
	secret, err := h.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		found = false
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.BackupTargetSecretName,
				Namespace: util.LonghornSystemNamespaceName,
			},
		}

		newSecret.StringData = setBackupSecret(target)
		if _, err = h.secrets.Create(newSecret); err != nil {
			return err
		}
	}

	if found {
		secretCpy := secret.DeepCopy()
		secretCpy.StringData = setBackupSecret(target)

		if !reflect.DeepEqual(secret.StringData, secretCpy.StringData) {
			if _, err := h.secrets.Update(secretCpy); err != nil {
				return err
			}
		}
	}

	return h.updateLonghornBackupTargetSecretSetting(target)
}

func (h *TargetHandler) updateLonghornBackupTargetSecretSetting(target *settings.BackupTarget) error {
	targetSecret, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornBackupTargetSecretSettingName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		if _, err := h.longhornSettings.Create(&longhornv1.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      longhornBackupTargetSecretSettingName,
				Namespace: util.LonghornSystemNamespaceName,
			},
			Setting: types.Setting{Value: util.BackupTargetSecretName},
		}); err != nil {
			return err
		}
		return nil
	}

	targetSecCpy := targetSecret.DeepCopy()
	targetSecCpy.Value = util.BackupTargetSecretName

	if targetSecret.Value != targetSecCpy.Value {
		if _, err := h.longhornSettings.Update(targetSecCpy); err != nil {
			return err
		}
	}

	return nil
}

func ConstructEndpoint(target *settings.BackupTarget) string {
	switch target.Type {
	case settings.S3BackupType:
		return fmt.Sprintf("s3://%s@%s/", target.BucketName, target.BucketRegion)
	case settings.NFSBackupType:
		// we allow users to input nfs:// prefix as optional
		return fmt.Sprintf("nfs://%s", strings.TrimPrefix(target.Endpoint, "nfs://"))
	default:
		return target.Endpoint
	}
}

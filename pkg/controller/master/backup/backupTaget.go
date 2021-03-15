package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/longhorn/backupstore/fsops"
	"github.com/minio/minio-go/v6"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterapiv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/rancher/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/rancher/harvester/pkg/settings"
)

const (
	backupTargetControllerName = "harvester-backup-target-controller"
	backupTargetSecretName     = "harvester-backup-target-secret"

	longhornBackupTargetSettingName       = "backup-target"
	longhornBackupTargetSecretSettingName = "backup-target-credential-secret"
)

// RegisterBackupTarget register the setting controller and validate the configured backup target server
func RegisterBackupTarget(ctx context.Context, management *config.Management, opts config.Options) error {
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	longhornSettings := management.LonghornFactory.Longhorn().V1beta1().Setting()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()

	backupTargetController := &TargetHandler{
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
	longhornSettings     ctllonghornv1.SettingClient
	longhornSettingCache ctllonghornv1.SettingCache
	secrets              ctlcorev1.SecretClient
	secretCache          ctlcorev1.SecretCache
	vms                  ctlkubevirtv1.VirtualMachineController
	settings             ctlharvesterv1.SettingClient
}

// OnBackupTargetChange handles backupTarget setting object on change
func (h *TargetHandler) OnBackupTargetChange(key string, setting *harvesterapiv1.Setting) (*harvesterapiv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != settings.BackupTargetSettingName {
		return nil, nil
	}

	target, err := decodeTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return setting, err
	}

	settingCpy := setting.DeepCopy()
	if harvesterapiv1.SettingConfigured.IsTrue(settingCpy) && target.Type == settings.S3BackupType &&
		(target.SecretAccessKey == "" || target.AccessKeyID == "") {
		return nil, nil
	}

	if target.Endpoint == "" {
		if err = h.updateLonghornTarget(target); err != nil {
			return nil, err
		}
		harvesterapiv1.SettingConfigured.False(settingCpy)
		harvesterapiv1.SettingConfigured.Reason(settingCpy, "Empty backup target")
	} else {
		target, err = validateTargetEndpoint(target)
		if err != nil {
			harvesterapiv1.SettingConfigured.False(settingCpy)
			harvesterapiv1.SettingConfigured.Reason(settingCpy, err.Error())
			_, err := h.settings.Update(settingCpy)
			return nil, err
		}

		if err = h.updateLonghornTarget(target); err != nil {
			return nil, err
		}

		if target.Type == settings.S3BackupType {
			if err = h.updateBackupTargetSecret(target); err != nil {
				return nil, err
			}
		}

		harvesterapiv1.SettingConfigured.True(settingCpy)
		harvesterapiv1.SettingConfigured.Reason(settingCpy, "")

		target.SecretAccessKey = ""
		target.AccessKeyID = ""
		settingCpy.Value, err = encodeTarget(target)
		if err != nil {
			return nil, err
		}
	}

	return h.settings.Update(settingCpy)
}

func (h *TargetHandler) updateLonghornTarget(backupTarget *settings.BackupTarget) error {
	endpoint := backupTarget.Endpoint
	target, err := h.longhornSettingCache.Get(LonghornSystemNameSpace, longhornBackupTargetSettingName)
	if err != nil {
		return err
	}

	targetCpy := target.DeepCopy()
	if backupTarget.Type == settings.S3BackupType {
		endpoint = fmt.Sprintf("s3://%s@%s/", backupTarget.BucketName, backupTarget.BucketRegion)
	}
	targetCpy.Value = endpoint

	if !reflect.DeepEqual(target, targetCpy) {
		_, err := h.longhornSettings.Update(targetCpy)
		return err
	}
	return nil
}

func setBackupSecret(target *settings.BackupTarget) map[string]string {
	return map[string]string{
		"AWS_ACCESS_KEY_ID":     target.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": target.SecretAccessKey,
		"AWS_ENDPOINTS":         target.Endpoint,
		"AWS_CERT":              target.Cert,
		"VIRTUAL_HOSTED_STYLE":  strconv.FormatBool(target.VirtualHostedStyle),
	}
}

func (h *TargetHandler) updateBackupTargetSecret(target *settings.BackupTarget) error {
	var found = true
	secret, err := h.secretCache.Get(LonghornSystemNameSpace, backupTargetSecretName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		found = false
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupTargetSecretName,
				Namespace: LonghornSystemNameSpace,
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

	targetSecret, err := h.longhornSettingCache.Get(LonghornSystemNameSpace, longhornBackupTargetSecretSettingName)
	if err != nil {
		return err
	}

	targetSecCpy := targetSecret.DeepCopy()
	targetSecCpy.Value = backupTargetSecretName

	if targetSecret.Value != targetSecCpy.Value {
		if _, err := h.longhornSettings.Update(targetSecCpy); err != nil {
			return err
		}
	}

	return nil
}

func validateTargetEndpoint(target *settings.BackupTarget) (*settings.BackupTarget, error) {
	// check whether have $ or , have been set in the BackupTarget
	regStr := `[\$\,]`
	reg := regexp.MustCompile(regStr)
	findStr := reg.FindAllString(target.Endpoint, -1)
	if len(findStr) != 0 {
		return nil, fmt.Errorf("value %s, contains %v", target.Endpoint, strings.Join(findStr, " or "))
	}

	switch target.Type {
	case settings.NFSBackupType:
		target.Endpoint = fmt.Sprintf("nfs://%s", strings.TrimPrefix(target.Endpoint, "nfs://"))
		return target, validateNFSBackupTarget(target.Endpoint)
	case settings.S3BackupType:
		err := validateS3BackupTarget(target)
		return target, err
	default:
		return nil, fmt.Errorf("unknown type of the backup target, currently only support NFS and S3")
	}
}

func validateNFSBackupTarget(destURL string) error {
	b := &StoreDriver{}
	b.FileSystemOperator = fsops.NewFileSystemOperator(b)

	u, err := url.Parse(destURL)
	if err != nil {
		return err
	}

	if u.Host == "" {
		return fmt.Errorf("NFS path must follow: nfs://server:/path/ format")
	}
	if u.Path == "" {
		return fmt.Errorf("cannot find nfs path")
	}

	b.serverPath = u.Host + u.Path
	b.mountDir = filepath.Join(MountDir, strings.TrimRight(strings.Replace(u.Host, ".", "_", -1), ":"), u.Path)
	if err := os.MkdirAll(b.mountDir, os.ModeDir|0700); err != nil {
		return fmt.Errorf("cannot create mount directory %v for NFS server", b.mountDir)
	}

	if err := b.mount(); err != nil {
		return fmt.Errorf("cannot mount nfs %v: %v", b.serverPath, err)
	}
	if _, err := b.List(""); err != nil {
		return fmt.Errorf("NFS path %v doesn't exist or is not a directory", b.serverPath)
	}

	b.destURL = KIND + "://" + b.serverPath
	logrus.Debugf("Loaded driver for %v", b.destURL)
	return b.unmount()
}

func validateS3BackupTarget(target *settings.BackupTarget) error {
	if target.AccessKeyID == "" || target.SecretAccessKey == "" {
		return fmt.Errorf("invalid s3 credentials, either accessKeyID or secretAccessKey is empty")
	}

	var secure bool
	var endpoint = target.Endpoint
	if strings.HasPrefix(endpoint, "http://") ||
		strings.HasPrefix(endpoint, "https://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return err
		}
		endpoint = u.Host
		secure = u.Scheme == "https"
	}

	client, err := minio.NewWithRegion(endpoint, target.AccessKeyID, target.SecretAccessKey, secure, target.BucketRegion)
	if err != nil {
		return err
	}

	exist, err := client.BucketExists(target.BucketName)
	if err != nil {
		return err
	}

	if !exist {
		return fmt.Errorf("bucket %s does not exist", target.BucketName)
	}

	return nil
}

func decodeTarget(value string) (*settings.BackupTarget, error) {
	setting := &settings.BackupTarget{}
	if err := json.Unmarshal([]byte(value), setting); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
	}

	return setting, nil
}

func encodeTarget(target *settings.BackupTarget) (string, error) {
	strTarget, err := json.Marshal(target)
	return string(strTarget), err
}

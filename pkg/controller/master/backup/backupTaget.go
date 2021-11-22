package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/longhorn/backupstore/fsops"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
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

	awss3EndpointKeyword = "amazonaws.com"
)

// RegisterBackupTarget register the setting controller and validate the configured backup target server
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

	target, err := decodeTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return setting, err
	}

	settingCpy := setting.DeepCopy()
	if target.Type == settings.S3BackupType && (target.SecretAccessKey == "" || target.AccessKeyID == "") {
		return nil, nil
	}

	target, err = h.validateTargetEndpoint(target)
	if err != nil {
		logrus.Errorf("invalid backup target, error: %s", err.Error())
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

	return h.updateBackupTargetSetting(settingCpy, target, err)
}

func (h *TargetHandler) updateBackupTargetSetting(setting *harvesterv1.Setting, target *settings.BackupTarget, err error) (*harvesterv1.Setting, error) {
	harvesterv1.SettingConfigured.SetError(setting, "", err)

	// reset the s3 credentials to prevent controller reconcile and not to expose secret key
	target.SecretAccessKey = ""
	target.AccessKeyID = ""
	setting.Value, err = encodeTarget(target)
	if err != nil {
		return nil, err
	}

	return h.settings.Update(setting)
}

func (h *TargetHandler) updateLonghornTarget(backupTarget *settings.BackupTarget) error {
	endpoint := backupTarget.Endpoint
	target, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornBackupTargetSettingName)
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

	targetSecret, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, longhornBackupTargetSecretSettingName)
	if err != nil {
		return err
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

func (h *TargetHandler) validateTargetEndpoint(target *settings.BackupTarget) (*settings.BackupTarget, error) {
	// check whether have $ or , have been set in the BackupTarget endpoint
	if len(target.Endpoint) > 0 {
		regStr := `[\$\,]`
		reg := regexp.MustCompile(regStr)
		findStr := reg.FindAllString(target.Endpoint, -1)
		if len(findStr) != 0 {
			return nil, fmt.Errorf("value %s, contains %v", target.Endpoint, strings.Join(findStr, " or "))
		}
	}

	switch target.Type {
	case settings.NFSBackupType:
		target.Endpoint = fmt.Sprintf("nfs://%s", strings.TrimPrefix(target.Endpoint, "nfs://"))
		return target, validateNFSBackupTarget(target.Endpoint)
	case settings.S3BackupType:
		err := h.validateS3BackupTarget(target)
		return target, err
	default:
		// When "Use the default value", the target.Type is "",
		//  if a dedicated type is added for "default value", check here
		defaultBackupTarget := settings.BackupTarget{}
		if reflect.DeepEqual(target, &defaultBackupTarget) {
			return target, nil
		}

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

func (h *TargetHandler) validateS3BackupTarget(target *settings.BackupTarget) error {
	credentials := credentials.StaticCredentialsProvider{
		Value: aws.Credentials{
			AccessKeyID:     target.AccessKeyID,
			SecretAccessKey: target.SecretAccessKey,
		},
	}

	endpointResolver := aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
		if target.Endpoint != "" {
			return aws.Endpoint{
				URL:           target.Endpoint,
				SigningRegion: target.BucketRegion,
			}, nil
		}

		// returning EndpointNotFoundError will allow the service to fallback to it's default resolution
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	var ca io.Reader
	if target.Cert != "" {
		ca = strings.NewReader(target.Cert)
	}
	cfg, err := awsconfig.LoadDefaultConfig(h.ctx, awsconfig.WithCredentialsProvider(credentials),
		awsconfig.WithEndpointResolver(endpointResolver),
		awsconfig.WithCustomCABundle(ca),
		awsconfig.WithDefaultRegion(target.BucketRegion))
	if err != nil {
		return err
	}

	// create a s3 service client
	client := s3.NewFromConfig(cfg)

	// client.HeadBucket is the right way to check if s3 bucket is existing
	// we also support using minio for the replacement of s3
	// as per the test, minio is not working well with HeadBucket
	// aws api will format URL like: http://backupbucket.minioip:9000, it gets DNS lookup fail error
	// if we skip the URL in endpointResolver, the "api error Forbidden: Forbidden" is returned from minio server
	//
	// in harvester web UI, when setting the backup target
	// for aws s3: the endpoint will normally be empty, aws api will format it;
	// when user input it, it will be like:
	// s3 in a virtual-hosted–style URL:
	//   http://bucket.s3.amazonaws.com
	//   http://bucket.s3-aws-region.amazonaws.com.
	// s3 in a path-style URL:
	//   http://s3.amazonaws.com/bucket
	//   http://s3-aws-region.amazonaws.com/bucket

	// for minio (s3 replacement): the endpoint is required, like
	//   https://minio-service.default:9000
	//   http://127.0.0.1:9000
	if target.Endpoint != "" && !strings.Contains(target.Endpoint, awss3EndpointKeyword) {
		// s3 replacement, like minio
		output, err := client.ListBuckets(h.ctx, &s3.ListBucketsInput{})
		if err != nil {
			return err
		}

		for _, b := range output.Buckets {
			if *b.Name == target.BucketName {
				return nil
			}
		}

		return fmt.Errorf("bucket %s does not exist", target.BucketName)
	} else {
		// aws s3
		headBucketInput := s3.HeadBucketInput{
			Bucket: &target.BucketName,
		}

		_, err = client.HeadBucket(h.ctx, &headBucketInput)
		if err != nil {
			return err
		}

		return nil
	}
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

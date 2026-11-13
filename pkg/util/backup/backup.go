package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/longhorn/backupstore"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	VMImageMetadataFolderPath = "harvester/vmimages/"
	// The webhook timeout is 10 seconds, so we can't set too long timeout here.
	ConnectBackupStoreTimeout = 8 * time.Second
)

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

func CurrentTarget() (*settings.BackupTarget, error) {
	return settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
}

func GetBackupStoreDriver(secretCache ctlcorev1.SecretCache, target *settings.BackupTarget) (backupstore.BackupStoreDriver, error) {
	if target.Type == settings.S3BackupType {
		secret, err := secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
		if err != nil {
			return nil, err
		}
		os.Setenv(util.AWSAccessKey, string(secret.Data[util.AWSAccessKey]))
		os.Setenv(util.AWSSecretKey, string(secret.Data[util.AWSSecretKey]))
		os.Setenv(util.AWSEndpoints, string(secret.Data[util.AWSEndpoints]))
		os.Setenv(util.AWSCERT, string(secret.Data[util.AWSCERT]))
	}

	endpoint := ConstructEndpoint(target)

	// There might be a goroutine leak if the driver doesn't end properly,
	// Although we can pass ctx, but the underlying driver implementation doesn't support it.
	// So we should be careful when using GetBackupStoreDriver function.
	bsDriver, err := util.RunWithTimeoutAndResult(ConnectBackupStoreTimeout, func(_ context.Context) (backupstore.BackupStoreDriver, error) {
		return backupstore.GetBackupStoreDriver(endpoint)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to backup target, reason: %w", err)
	}
	return bsDriver, nil
}

func IsBackupTargetSame(statusBackupTarget *harvesterv1.BackupTarget, target *settings.BackupTarget) bool {
	if (statusBackupTarget == nil && target != nil) || (statusBackupTarget != nil && target == nil) {
		return false
	}
	if statusBackupTarget == nil {
		return true
	}
	if statusBackupTarget.RepositoryID != "" {
		return statusBackupTarget.RepositoryID == RepositoryID(target)
	}
	return statusBackupTarget.Endpoint == target.Endpoint &&
		statusBackupTarget.BucketName == target.BucketName &&
		statusBackupTarget.BucketRegion == target.BucketRegion
}

// CheckTarget validates a data-mover backup before using the current target's credentials.
func CheckTarget(status *harvesterv1.BackupTarget) error {
	if _, err := TargetFromStatus(status); err != nil {
		return err
	}
	target, err := CurrentTarget()
	if err != nil {
		return err
	}
	if !IsBackupTargetSame(status, target) {
		return fmt.Errorf("VMBackup backup target does not match the configured backup target")
	}
	return nil
}

// RepositoryID returns a stable, non-secret identifier for a backup target.
// Authentication and TLS material are intentionally excluded so IAM
// credentials and certificates can be rotated without changing repository
// identity.
func RepositoryID(target *settings.BackupTarget) string {
	if target == nil || target.IsDefaultBackupTarget() {
		return ""
	}

	identity := strings.Join([]string{
		string(target.Type),
		strings.TrimRight(target.Endpoint, "/"),
		target.BucketName,
		target.BucketRegion,
		fmt.Sprintf("%t", target.VirtualHostedStyle),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RepositoryPassword derives an initial engine-specific password from the
// repository ID and S3 credentials. Once initialized, the stored password is
// preserved for that repository even when its S3 credentials change.
func RepositoryPassword(
	repositoryID string, backupType harvesterv1.BackupType, credentials map[string][]byte,
) (string, error) {
	if repositoryID == "" {
		return "", fmt.Errorf("repository ID is empty")
	}
	if !backupType.IsDataMover() {
		return "", fmt.Errorf("unsupported data-mover backup type %q", backupType)
	}
	accessKey, secretKey := credentials[util.AWSAccessKey], credentials[util.AWSSecretKey]
	if len(accessKey) == 0 {
		return "", fmt.Errorf("S3 access key ID is empty")
	}
	if len(secretKey) == 0 {
		return "", fmt.Errorf("S3 secret access key is empty")
	}

	identity := strings.Join([]string{string(backupType), repositoryID, string(accessKey)}, "\x00")
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(identity))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// BackupTargetStatus builds the non-secret target information persisted in a
// VMBackup status and its remote metadata.
func BackupTargetStatus(target *settings.BackupTarget) *harvesterv1.BackupTarget {
	if target == nil {
		return nil
	}
	return &harvesterv1.BackupTarget{
		Type:               string(target.Type),
		Endpoint:           target.Endpoint,
		BucketName:         target.BucketName,
		BucketRegion:       target.BucketRegion,
		VirtualHostedStyle: target.VirtualHostedStyle,
		RepositoryID:       RepositoryID(target),
	}
}

// TargetFromStatus reconstructs the repository location recorded by a
// data-mover VMBackup.
func TargetFromStatus(status *harvesterv1.BackupTarget) (*settings.BackupTarget, error) {
	if status == nil {
		return nil, fmt.Errorf("VMBackup does not record a backup target")
	}
	if status.Type == "" {
		return nil, fmt.Errorf("VMBackup does not record a backup target type")
	}
	if status.RepositoryID == "" {
		return nil, fmt.Errorf("VMBackup does not record a repository ID")
	}

	targetType := settings.TargetType(status.Type)
	target := &settings.BackupTarget{
		Type:               targetType,
		Endpoint:           status.Endpoint,
		BucketName:         status.BucketName,
		BucketRegion:       status.BucketRegion,
		VirtualHostedStyle: status.VirtualHostedStyle,
	}
	if status.RepositoryID != RepositoryID(target) {
		return nil, fmt.Errorf("VMBackup repository ID does not match its recorded backup target")
	}
	return target, nil
}

func GetVMImageMetadataFilePath(vmImageNamespace, vmImageName string) string {
	return filepath.Join(VMImageMetadataFolderPath, vmImageNamespace, fmt.Sprintf("%s.cfg", vmImageName))
}

func LHSnapToVSCName(lhSnapshotName string) string {
	return strings.Replace(lhSnapshotName, "snapshot", "snapcontent", 1)
}

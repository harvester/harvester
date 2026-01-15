package backup

import (
	"context"
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
	return statusBackupTarget.Endpoint == target.Endpoint && statusBackupTarget.BucketName == target.BucketName && statusBackupTarget.BucketRegion == target.BucketRegion
}

func GetVMImageMetadataFilePath(vmImageNamespace, vmImageName string) string {
	return filepath.Join(VMImageMetadataFolderPath, vmImageNamespace, fmt.Sprintf("%s.cfg", vmImageName))
}

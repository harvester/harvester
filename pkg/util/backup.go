package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/longhorn/backupstore"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	VMImageMetadataFolderPath = "harvester/vmimages/"
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
		secret, err := secretCache.Get(LonghornSystemNamespaceName, BackupTargetSecretName)
		if err != nil {
			return nil, err
		}
		os.Setenv(AWSAccessKey, string(secret.Data[AWSAccessKey]))
		os.Setenv(AWSSecretKey, string(secret.Data[AWSSecretKey]))
		os.Setenv(AWSEndpoints, string(secret.Data[AWSEndpoints]))
		os.Setenv(AWSCERT, string(secret.Data[AWSCERT]))
	}

	endpoint := ConstructEndpoint(target)
	bsDriver, err := backupstore.GetBackupStoreDriver(endpoint)
	if err != nil {
		return nil, err
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

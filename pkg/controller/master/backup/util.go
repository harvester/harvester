package backup

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"

	ctllonghornv2 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/util"
)

func getVMBackupMetadataFilePath(vmBackupNamespace, vmBackupName string) string {
	return filepath.Join(vmBackupMetadataFolderPath, vmBackupNamespace, fmt.Sprintf("%s.cfg", vmBackupName))
}

func getBackupTargetHash(value string) (string, error) {
	hash := sha256.New224()
	if _, err := io.Copy(hash, io.MultiReader(strings.NewReader(value))); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func checkLHBackup(backupCache ctllonghornv2.BackupCache, name string) (string, error) {
	backup, err := backupCache.Get(util.LonghornSystemNamespaceName, name)
	if err != nil {
		return "", err
	}

	if backup.Status.State != lhv1beta2.BackupStateCompleted {
		return fmt.Sprintf("backup %s is not completed", name), nil
	}

	if backup.DeletionTimestamp != nil {
		return fmt.Sprintf("backup %s is being deleted", name), nil
	}
	return "", nil
}

func checkStorageClass(storageClassCache ctlstoragev1.StorageClassCache, name string) error {
	storageClass, err := storageClassCache.Get(name)
	if err != nil {
		return err
	}

	if storageClass.DeletionTimestamp != nil {
		return fmt.Errorf("storage class %s is being deleted", name)
	}
	return nil
}

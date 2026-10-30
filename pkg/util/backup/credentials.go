package backup

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func CredentialData(
	cache ctlcorev1.SecretCache, target *harvesterv1.BackupTarget,
	backupType harvesterv1.BackupType, passwordKey string,
) (map[string][]byte, error) {
	if target == nil {
		return nil, fmt.Errorf("VMBackup does not record a backup target")
	}
	password, err := RepositoryPassword(target.RepositoryID, backupType)
	if err != nil {
		return nil, err
	}
	source, err := cache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		util.AWSAccessKey: source.Data[util.AWSAccessKey],
		util.AWSSecretKey: source.Data[util.AWSSecretKey],
		passwordKey:       []byte(password),
	}, nil
}

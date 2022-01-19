package indexeres

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/webhook/clients"
)

const (
	VMBackupBySourceUIDIndex = "harvesterhci.io/vmbackup-by-source-uid"
)

func RegisterIndexers(clients *clients.Clients) {
	vmBackupCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmBackupCache.AddIndexer(VMBackupBySourceUIDIndex, vmBackupBySourceUID)
}

func vmBackupBySourceUID(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if obj.Status != nil && obj.Status.SourceUID != nil {
		return []string{string(*obj.Status.SourceUID)}, nil
	}
	return []string{}, nil
}

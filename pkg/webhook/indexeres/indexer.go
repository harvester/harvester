package indexeres

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/webhook/clients"
)

const (
	VMBackupBySourceUIDIndex          = "harvesterhci.io/vmbackup-by-source-uid"
	VMRestoreByTargetNamespaceAndName = "harvesterhci.io/vmrestore-by-target-namespace-and-name"
)

func RegisterIndexers(clients *clients.Clients) {
	vmBackupCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmRestoreCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache()
	vmBackupCache.AddIndexer(VMBackupBySourceUIDIndex, vmBackupBySourceUID)
	vmRestoreCache.AddIndexer(VMRestoreByTargetNamespaceAndName, vmRestoreByTargetNamespaceAndName)
}

func vmBackupBySourceUID(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if obj.Status != nil && obj.Status.SourceUID != nil {
		return []string{string(*obj.Status.SourceUID)}, nil
	}
	return []string{}, nil
}

func vmRestoreByTargetNamespaceAndName(obj *harvesterv1.VirtualMachineRestore) ([]string, error) {
	if obj == nil {
		return []string{}, nil
	}
	return []string{fmt.Sprintf("%s-%s", obj.Namespace, obj.Spec.Target.Name)}, nil
}

package indexeres

import (
	"fmt"

	longhorntypes "github.com/longhorn/longhorn-manager/types"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/webhook/clients"
)

const (
	VMBackupBySourceUIDIndex              = "harvesterhci.io/vmbackup-by-source-uid"
	VMRestoreByTargetNamespaceAndName     = "harvesterhci.io/vmrestore-by-target-namespace-and-name"
	VMRestoreByVMBackupNamespaceAndName   = "harvesterhci.io/vmrestore-by-vmbackup-namespace-and-name"
	VMBackupSnapshotByPVCNamespaceAndName = "harvesterhci.io/vmbackup-snapshot-by-pvc-namespace-and-name"
	ImageByExportSourcePVCIndex           = "harvesterhci.io/image-by-export-source-pvc"
)

func RegisterIndexers(clients *clients.Clients) {
	vmBackupCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmBackupCache.AddIndexer(VMBackupBySourceUIDIndex, vmBackupBySourceUID)
	vmBackupCache.AddIndexer(VMBackupSnapshotByPVCNamespaceAndName, vmBackupSnapshotByPVCNamespaceAndName)

	vmRestoreCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache()
	vmRestoreCache.AddIndexer(VMRestoreByTargetNamespaceAndName, vmRestoreByTargetNamespaceAndName)
	vmRestoreCache.AddIndexer(VMRestoreByVMBackupNamespaceAndName, vmRestoreByVMBackupNamespaceAndName)

	podCache := clients.CoreFactory.Core().V1().Pod().Cache()
	podCache.AddIndexer(indexeres.PodByVMNameIndex, indexeres.PodByVMName)

	vmImageInformer := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache()
	vmImageInformer.AddIndexer(ImageByExportSourcePVCIndex, imageByExportSourcePVC)
}

func vmBackupBySourceUID(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if obj.Status != nil && obj.Status.SourceUID != nil {
		return []string{string(*obj.Status.SourceUID)}, nil
	}
	return []string{}, nil
}

func vmBackupSnapshotByPVCNamespaceAndName(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if obj.Spec.Type == harvesterv1.Backup || obj.Status == nil {
		return []string{}, nil
	}

	result := make([]string, 0, len(obj.Status.VolumeBackups))
	for _, volumeBackup := range obj.Status.VolumeBackups {
		pvc := volumeBackup.PersistentVolumeClaim
		result = append(result, fmt.Sprintf("%s/%s", pvc.ObjectMeta.Namespace, pvc.ObjectMeta.Name))
	}
	return result, nil
}

func vmRestoreByTargetNamespaceAndName(obj *harvesterv1.VirtualMachineRestore) ([]string, error) {
	if obj == nil {
		return []string{}, nil
	}
	return []string{fmt.Sprintf("%s-%s", obj.Namespace, obj.Spec.Target.Name)}, nil
}

func vmRestoreByVMBackupNamespaceAndName(obj *harvesterv1.VirtualMachineRestore) ([]string, error) {
	if obj == nil {
		return []string{}, nil
	}
	return []string{fmt.Sprintf("%s-%s", obj.Spec.VirtualMachineBackupNamespace, obj.Spec.VirtualMachineBackupName)}, nil
}

func imageByExportSourcePVC(obj *harvesterv1.VirtualMachineImage) ([]string, error) {
	if obj.Spec.SourceType != longhorntypes.LonghornLabelExportFromVolume ||
		obj.Spec.PVCNamespace == "" || obj.Spec.PVCName == "" {
		return nil, nil
	}

	return []string{fmt.Sprintf("%s/%s", obj.Spec.PVCNamespace, obj.Spec.PVCName)}, nil
}

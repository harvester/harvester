package indexeres

import (
	"fmt"
	"strconv"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	"github.com/harvester/harvester/pkg/webhook/clients"
)

const (
	VMBackupBySourceUIDIndex              = "harvesterhci.io/vmbackup-by-source-uid"
	VMRestoreByTargetNamespaceAndName     = "harvesterhci.io/vmrestore-by-target-namespace-and-name"
	VMRestoreByVMBackupNamespaceAndName   = "harvesterhci.io/vmrestore-by-vmbackup-namespace-and-name"
	VMBackupSnapshotByPVCNamespaceAndName = "harvesterhci.io/vmbackup-snapshot-by-pvc-namespace-and-name"
	VolumeByReplicaCountIndex             = "harvesterhci.io/volume-by-replica-count"
	ImageByExportSourcePVCIndex           = "harvesterhci.io/image-by-export-source-pvc"
	ScheduleVMBackupBySourceVM            = "harvesterhci.io/svmbackup-by-source-vm"
	ScheduleVMBackupByCronGranularity     = "harvesterhci.io/svmbackup-by-cron-granularity"
	ImageByStorageClass                   = "harvesterhci.io/image-by-storage-class"
	VMInstanceMigrationByVM               = "harvesterhci.io/vmim-by-vm"
)

func RegisterIndexers(clients *clients.Clients) {
	vmBackupCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmBackupCache.AddIndexer(VMBackupBySourceUIDIndex, vmBackupBySourceUID)
	vmBackupCache.AddIndexer(VMBackupSnapshotByPVCNamespaceAndName, vmBackupSnapshotByPVCNamespaceAndName)

	vmRestoreCache := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache()
	vmRestoreCache.AddIndexer(VMRestoreByTargetNamespaceAndName, vmRestoreByTargetNamespaceAndName)
	vmRestoreCache.AddIndexer(VMRestoreByVMBackupNamespaceAndName, vmRestoreByVMBackupNamespaceAndName)

	podCache := clients.CoreFactory.Core().V1().Pod().Cache()
	podCache.AddIndexer(indexeresutil.PodByVMNameIndex, indexeresutil.PodByVMName)

	volumeCache := clients.LonghornFactory.Longhorn().V1beta2().Volume().Cache()
	volumeCache.AddIndexer(VolumeByReplicaCountIndex, VolumeByReplicaCount)

	vmImageInformer := clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache()
	vmImageInformer.AddIndexer(ImageByExportSourcePVCIndex, imageByExportSourcePVC)
	vmImageInformer.AddIndexer(ImageByStorageClass, imageByStorageClass)

	vmInformer := clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()
	vmInformer.AddIndexer(indexeresutil.VMByPVCIndex, indexeresutil.VMByPVC)

	svmBackupCache := clients.HarvesterFactory.Harvesterhci().V1beta1().ScheduleVMBackup().Cache()
	svmBackupCache.AddIndexer(ScheduleVMBackupBySourceVM, scheduleVMBackupBySourceVM)
	svmBackupCache.AddIndexer(ScheduleVMBackupByCronGranularity, scheduleVMBackupByCronGranularity)

	scInformer := clients.StorageFactory.Storage().V1().StorageClass().Cache()
	scInformer.AddIndexer(indexeresutil.StorageClassBySecretIndex, indexeresutil.StorageClassBySecret)

	vmimCache := clients.KubevirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration().Cache()
	vmimCache.AddIndexer(VMInstanceMigrationByVM, vmInstanceMigrationByVM)
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

func VolumeByReplicaCount(obj *lhv1beta2.Volume) ([]string, error) {
	replicaCount := strconv.Itoa(obj.Spec.NumberOfReplicas)
	return []string{replicaCount}, nil
}

func imageByExportSourcePVC(obj *harvesterv1.VirtualMachineImage) ([]string, error) {
	if obj.Spec.SourceType != longhorntypes.LonghornLabelExportFromVolume ||
		obj.Spec.PVCNamespace == "" || obj.Spec.PVCName == "" {
		return nil, nil
	}

	return []string{fmt.Sprintf("%s/%s", obj.Spec.PVCNamespace, obj.Spec.PVCName)}, nil
}

func scheduleVMBackupBySourceVM(obj *harvesterv1.ScheduleVMBackup) ([]string, error) {
	return []string{fmt.Sprintf("%s/%s", obj.Namespace, obj.Spec.VMBackupSpec.Source.Name)}, nil
}

func scheduleVMBackupByCronGranularity(obj *harvesterv1.ScheduleVMBackup) ([]string, error) {
	if obj == nil {
		return []string{}, nil
	}

	granularity, err := util.GetCronGranularity(obj)
	if err != nil {
		return []string{}, err
	}

	return []string{granularity.String()}, nil
}

func imageByStorageClass(obj *harvesterv1.VirtualMachineImage) ([]string, error) {
	sc, ok := obj.Annotations[util.AnnotationStorageClassName]
	if !ok {
		return []string{}, nil
	}
	return []string{sc}, nil
}

func vmInstanceMigrationByVM(obj *kubevirtv1.VirtualMachineInstanceMigration) ([]string, error) {
	return []string{fmt.Sprintf("%s/%s", obj.Namespace, obj.Spec.VMIName)}, nil
}

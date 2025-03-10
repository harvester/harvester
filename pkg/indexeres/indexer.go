package indexeres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/steve/pkg/server"
	corev1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

const (
	PVCByDataSourceVolumeSnapshotIndex = "harvesterhci.io/pvc-by-data-source-volume-snapshot"
	PodByNodeNameIndex                 = "harvesterhci.io/pod-by-nodename"
	PodByPVCIndex                      = "harvesterhci.io/pod-by-pvc"
	VolumeByNodeIndex                  = "harvesterhci.io/volume-by-node"
	VMBackupBySourceVMUIDIndex         = "harvesterhci.io/vmbackup-by-source-vm-uid"
	VMBackupBySourceVMNameIndex        = "harvesterhci.io/vmbackup-by-source-vm-name"
	VMTemplateVersionByImageIDIndex    = "harvesterhci.io/vmtemplateversion-by-image-id"
	VolumeSnapshotBySourcePVCIndex     = "harvesterhci.io/volumesnapshot-by-source-pvc"
)

func Setup(ctx context.Context, _ *server.Server, _ *server.Controllers, _ config.Options) error {
	scaled := config.ScaledWithContext(ctx)
	management := scaled.Management

	pvcInformer := management.CoreFactory.Core().V1().PersistentVolumeClaim().Cache()
	pvcInformer.AddIndexer(PVCByDataSourceVolumeSnapshotIndex, pvcByDataSourceVolumeSnapshot)

	podInformer := management.CoreFactory.Core().V1().Pod().Cache()
	podInformer.AddIndexer(PodByNodeNameIndex, PodByNodeName)
	podInformer.AddIndexer(PodByPVCIndex, PodByPVC)
	podInformer.AddIndexer(indexeresutil.PodByVMNameIndex, indexeresutil.PodByVMName)

	volumeInformer := management.LonghornFactory.Longhorn().V1beta2().Volume().Cache()
	volumeInformer.AddIndexer(VolumeByNodeIndex, VolumeByNodeName)

	volumeSnapshotInformer := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot().Cache()
	volumeSnapshotInformer.AddIndexer(VolumeSnapshotBySourcePVCIndex, volumeSnapshotBySourcePVC)

	vmBackupInformer := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmBackupInformer.AddIndexer(VMBackupBySourceVMNameIndex, VMBackupBySourceVMName)
	vmBackupInformer.AddIndexer(VMBackupBySourceVMUIDIndex, VMBackupBySourceVMUID)

	vmTemplateVersionInformer := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache()
	vmTemplateVersionInformer.AddIndexer(VMTemplateVersionByImageIDIndex, VMTemplateVersionByImageID)

	vmInformer := management.VirtFactory.Kubevirt().V1().VirtualMachine().Cache()
	vmInformer.AddIndexer(indexeresutil.VMByPVCIndex, indexeresutil.VMByPVC)

	scInformer := management.StorageFactory.Storage().V1().StorageClass().Cache()
	scInformer.AddIndexer(indexeresutil.StorageClassBySecretIndex, indexeresutil.StorageClassBySecret)
	return nil
}

func PodByNodeName(obj *corev1.Pod) ([]string, error) {
	return []string{obj.Spec.NodeName}, nil
}

func PodByPVC(obj *corev1.Pod) ([]string, error) {
	pvcNames := []string{}
	for _, volume := range obj.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, fmt.Sprintf("%s/%s", obj.Namespace, volume.PersistentVolumeClaim.ClaimName))
		}
	}
	return pvcNames, nil
}

func pvcByDataSourceVolumeSnapshot(obj *corev1.PersistentVolumeClaim) ([]string, error) {
	if obj.Spec.DataSource == nil || obj.Spec.DataSource.Kind != "VolumeSnapshot" {
		return []string{}, nil
	}
	return []string{fmt.Sprintf("%s/%s", obj.Namespace, obj.Spec.DataSource.Name)}, nil
}

func VMBackupBySourceVMUID(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	if obj.Status == nil || obj.Status.SourceUID == nil {
		return []string{}, nil
	}
	return []string{string(*obj.Status.SourceUID)}, nil
}

func VMBackupBySourceVMName(obj *harvesterv1.VirtualMachineBackup) ([]string, error) {
	return []string{obj.Spec.Source.Name}, nil
}

func VMTemplateVersionByImageID(obj *harvesterv1.VirtualMachineTemplateVersion) ([]string, error) {
	volumeClaimTemplateStr, ok := obj.Spec.VM.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplateStr == "" {
		return []string{}, nil
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplateStr), &volumeClaimTemplates); err != nil {
		// an IndexFunc should never return an error as this would cause the cache
		// to panic. Therefore we just log the error and return an empty result.
		logrus.WithFields(logrus.Fields{
			"annotation": util.AnnotationVolumeClaimTemplates,
			"name":       obj.Name,
			"namespace":  obj.Namespace,
			"err":        err.Error(),
		}).Error("can't unmarshal JSON data")
		return []string{}, nil
	}

	imageIDs := []string{}
	for _, volumeClaimTemplate := range volumeClaimTemplates {
		imageID, ok := volumeClaimTemplate.Annotations[util.AnnotationImageID]
		if !ok || imageID == "" {
			continue
		}
		imageIDs = append(imageIDs, imageID)
	}
	return imageIDs, nil
}

func volumeSnapshotBySourcePVC(obj *snapshotv1.VolumeSnapshot) ([]string, error) {
	if obj.Spec.Source.PersistentVolumeClaimName == nil {
		return []string{}, nil
	}
	return []string{fmt.Sprintf("%s/%s", obj.Namespace, *obj.Spec.Source.PersistentVolumeClaimName)}, nil
}

func VolumeByNodeName(obj *lhv1beta2.Volume) ([]string, error) {
	return []string{obj.Spec.NodeID}, nil
}

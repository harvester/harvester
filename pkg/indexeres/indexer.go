package indexeres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rancher/steve/pkg/server"
	corev1 "k8s.io/api/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

const (
	PVCByVMIndex                    = "harvesterhci.io/pvc-by-vm-index"
	VMByNetworkIndex                = "vm.harvesterhci.io/vm-by-network"
	PodByNodeNameIndex              = "harvesterhci.io/pod-by-nodename"
	VMBackupBySourceVMUIDIndex      = "harvesterhci.io/vmbackup-by-source-vm-uid"
	VMBackupBySourceVMNameIndex     = "harvesterhci.io/vmbackup-by-source-vm-name"
	VMTemplateVersionByImageIDIndex = "harvesterhci.io/vmtemplateversion-by-image-id"
)

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers, options config.Options) error {
	scaled := config.ScaledWithContext(ctx)
	management := scaled.Management

	pvcInformer := management.CoreFactory.Core().V1().PersistentVolumeClaim().Cache()
	pvcInformer.AddIndexer(PVCByVMIndex, pvcByVM)

	podInformer := management.CoreFactory.Core().V1().Pod().Cache()
	podInformer.AddIndexer(PodByNodeNameIndex, PodByNodeName)

	vmBackupInformer := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()
	vmBackupInformer.AddIndexer(VMBackupBySourceVMNameIndex, VMBackupBySourceVMName)
	vmBackupInformer.AddIndexer(VMBackupBySourceVMUIDIndex, VMBackupBySourceVMUID)

	vmTemplateVersionInformer := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache()
	vmTemplateVersionInformer.AddIndexer(VMTemplateVersionByImageIDIndex, VMTemplateVersionByImageID)
	return nil
}

func pvcByVM(obj *corev1.PersistentVolumeClaim) ([]string, error) {
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema owners from PVC %s's annotation: %w", obj.Name, err)
	}
	return annotationSchemaOwners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind()), nil
}

func VMByNetwork(obj *kubevirtv1.VirtualMachine) ([]string, error) {
	networks := obj.Spec.Template.Spec.Networks
	networkNameList := make([]string, 0, len(networks))
	for _, network := range networks {
		if network.NetworkSource.Multus == nil {
			continue
		}
		networkNameList = append(networkNameList, network.NetworkSource.Multus.NetworkName)
	}
	return networkNameList, nil
}

func PodByNodeName(obj *corev1.Pod) ([]string, error) {
	return []string{obj.Spec.NodeName}, nil
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
		return []string{}, fmt.Errorf("can't unmarshal %s, err: %w", util.AnnotationVolumeClaimTemplates, err)
	}

	imageIds := []string{}
	for _, volumeClaimTemplate := range volumeClaimTemplates {
		imageID, ok := volumeClaimTemplate.Annotations[util.AnnotationImageID]
		if !ok || imageID == "" {
			continue
		}
		imageIds = append(imageIds, imageID)
	}
	return imageIds, nil
}

package cdi

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/common"
)

func (b *Backend) initializeClone(vmImg *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	// Verify the source image are ready for cloning
	security := vmImg.Spec.SecurityParameters
	source, err := b.vmio.GetVMImageObj(security.SourceImageNamespace, security.SourceImageName)
	if err != nil {
		return vmImg, fmt.Errorf("failed to get source image: %w", err)
	}
	if source.Status.VirtualSize <= 0 {
		return vmImg, fmt.Errorf("source image %s/%s has no virtual size", source.Namespace, source.Name)
	}
	if source.DeletionTimestamp != nil || !harvesterv1.ImageImported.IsTrue(source) {
		return vmImg, fmt.Errorf("source image %s/%s is not ready", source.Namespace, source.Name)
	}

	// Verify the source PVC are ready for cloning
	sourcePVC, err := b.pvcCache.Get(source.Namespace, source.Name)
	if err != nil {
		return vmImg, fmt.Errorf("failed to get source image PVC: %w", err)
	}
	if sourcePVC.DeletionTimestamp != nil || sourcePVC.Status.Phase != corev1.ClaimBound {
		return vmImg, fmt.Errorf("source image PVC %s/%s is not ready", sourcePVC.Namespace, sourcePVC.Name)
	}
	// Longhorn use a 2MiB round up, so the actual allocated size may be slightly larger than requested.
	sourcePVCSize := max(sourcePVC.Spec.Resources.Requests.Storage().Value(), sourcePVC.Status.Capacity.Storage().Value())
	if sourcePVCSize <= 0 {
		return vmImg, fmt.Errorf("source image PVC has no storage capacity")
	}

	vmImg, err = b.initializeCloneImageSize(vmImg, source.Status.VirtualSize, max(source.Status.VirtualSize, sourcePVCSize))
	if err != nil {
		return vmImg, err
	}
	targetSC, err := b.scClient.Get(vmImg.Spec.TargetStorageClassName, metav1.GetOptions{})
	if err != nil {
		return vmImg, fmt.Errorf("failed to get target StorageClass: %w", err)
	}
	dv, err := generateCloneDataVolume(vmImg, sourcePVC, targetSC)
	if err != nil {
		return vmImg, err
	}
	if _, err := b.dataVolumeClient.Create(dv); err != nil && !apierrors.IsAlreadyExists(err) {
		return vmImg, fmt.Errorf("failed to create image clone DataVolume: %w", err)
	}
	return vmImg, nil
}

func (b *Backend) initializeCloneImageSize(vmImg *harvesterv1.VirtualMachineImage, virtualSize, size int64) (*harvesterv1.VirtualMachineImage, error) {
	if vmImg.Status.Size == size && vmImg.Status.VirtualSize == virtualSize {
		return vmImg, nil
	}
	updated, err := b.vmio.UpdateVirtualSizeAndSize(vmImg, virtualSize, size)
	if err != nil {
		return vmImg, err
	}
	return updated, nil
}

func generateCloneDataVolume(vmImg *harvesterv1.VirtualMachineImage, sourcePVC *corev1.PersistentVolumeClaim, targetSC *storagev1.StorageClass) (*cdiv1.DataVolume, error) {
	storage, err := generateDVTargetStorage(vmImg)
	if err != nil {
		return nil, err
	}
	annotations := GenerateDVAnnotations(targetSC)
	// CSI cloning across encryption boundaries (e.g., plain to encrypted or vice-versa) is currently not supported in Longhorn.
	// This is because Longhorn volumes can only be encrypted upon creation, and encrypting an existing volume isn't supported in Longhorn.
	// Therefore, CDI must not use the populator for cloning encrypted Longhorn volumes. Here we add the annotation
	// to force copy strategy to "host-assisted" to workaround the issue.
	// see https://github.com/longhorn/longhorn/issues/14222 for details
	annotations[cdicommon.AnnUsePopulator] = "false"
	var boolTrue = true
	return &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vmImg.Name,
			Namespace:   vmImg.Namespace,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         common.HarvesterAPIV1Beta1,
					Kind:               common.VMImageKind,
					Name:               vmImg.Name,
					UID:                vmImg.UID,
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Name:      sourcePVC.Name,
					Namespace: sourcePVC.Namespace,
				}},
			Storage: storage,
		},
	}, nil
}

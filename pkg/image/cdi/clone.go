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

func (b *Backend) initializeClone(
	vmImg *harvesterv1.VirtualMachineImage,
) (*harvesterv1.VirtualMachineImage, error) {
	source, err := b.getCloneSource(vmImg)
	if err != nil {
		return vmImg, err
	}
	sourcePVC, pvcSize, err := b.getClonePVC(source)
	if err != nil {
		return vmImg, err
	}

	virtualSize := b.vmio.GetVirtualSize(source)
	size := max(virtualSize, pvcSize)
	vmImg, err = b.updateCloneSize(vmImg, virtualSize, size)
	if err != nil {
		return vmImg, err
	}
	return vmImg, b.createCloneDV(vmImg, sourcePVC)
}

func (b *Backend) getCloneSource(
	vmImg *harvesterv1.VirtualMachineImage,
) (*harvesterv1.VirtualMachineImage, error) {
	namespace := b.vmio.GetSecuritySrcImgNamespace(vmImg)
	name := b.vmio.GetSecuritySrcImgName(vmImg)
	source, err := b.vmio.GetVMImageObj(namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get source image: %w", err)
	}
	namespace = b.vmio.GetNamespace(source)
	name = b.vmio.GetName(source)
	if b.vmio.GetVirtualSize(source) <= 0 {
		return nil, fmt.Errorf("source image %s/%s has no virtual size", namespace, name)
	}
	if source.DeletionTimestamp != nil || !b.vmio.IsImported(source) {
		return nil, fmt.Errorf("source image %s/%s is not ready", namespace, name)
	}
	return source, nil
}

func (b *Backend) getClonePVC(
	source *harvesterv1.VirtualMachineImage,
) (*corev1.PersistentVolumeClaim, int64, error) {
	pvc, err := b.pvcCache.Get(b.vmio.GetNamespace(source), b.vmio.GetName(source))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get source image PVC: %w", err)
	}
	if pvc.DeletionTimestamp != nil || pvc.Status.Phase != corev1.ClaimBound {
		return nil, 0, fmt.Errorf("source image PVC %s/%s is not ready",
			pvc.Namespace, pvc.Name)
	}

	// Longhorn rounds allocations up to 2 MiB, so capacity can exceed the request.
	size := max(
		pvc.Spec.Resources.Requests.Storage().Value(),
		pvc.Status.Capacity.Storage().Value(),
	)
	if size <= 0 {
		return nil, 0, fmt.Errorf("source image PVC has no storage capacity")
	}
	return pvc, size, nil
}

func (b *Backend) updateCloneSize(
	vmImg *harvesterv1.VirtualMachineImage,
	virtualSize, size int64,
) (*harvesterv1.VirtualMachineImage, error) {
	if vmImg.Status.Size == size && b.vmio.GetVirtualSize(vmImg) == virtualSize {
		return vmImg, nil
	}
	updated, err := b.vmio.UpdateVirtualSizeAndSize(vmImg, virtualSize, size)
	if err != nil {
		return vmImg, err
	}
	return updated, nil
}

func (b *Backend) createCloneDV(
	vmImg *harvesterv1.VirtualMachineImage,
	sourcePVC *corev1.PersistentVolumeClaim,
) error {
	targetSC, err := b.scClient.Get(b.vmio.GetStorageClassName(vmImg), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get target StorageClass: %w", err)
	}
	dv, err := generateCloneDataVolume(vmImg, sourcePVC, targetSC)
	if err != nil {
		return err
	}
	if _, err := b.dataVolumeClient.Create(dv); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create image clone DataVolume: %w", err)
	}
	return nil
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

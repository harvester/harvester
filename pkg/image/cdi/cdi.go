package cdi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
)

type Backend struct {
	ctx              context.Context
	dataVolumeClient ctlcdiv1.DataVolumeClient
	scClient         ctlstoragev1.StorageClassClient
	pvcCache         ctlcorev1.PersistentVolumeClaimCache
	vmio             common.VMIOperator
}

func GetBackend(ctx context.Context, dataVolumeClient ctlcdiv1.DataVolumeClient, scClient ctlstoragev1.StorageClassClient, pvcCache ctlcorev1.PersistentVolumeClaimCache, vmio common.VMIOperator) backend.Backend {
	return &Backend{
		ctx:              ctx,
		dataVolumeClient: dataVolumeClient,
		scClient:         scClient,
		pvcCache:         pvcCache,
		vmio:             vmio,
	}
}

func (b *Backend) Initialize(vmImg *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	// if dataVolume is already created, return
	created, err := b.isDataVolumeCreated(vmImg)
	if err != nil {
		return vmImg, fmt.Errorf("failed to check DataVolume: %v", err)
	}
	if created {
		return vmImg, nil
	}

	switch b.vmio.GetSourceType(vmImg) {
	case harvesterv1.VirtualMachineImageSourceTypeDownload:
		return b.initializeDownload(vmImg)
	case harvesterv1.VirtualMachineImageSourceTypeUpload:
		// do nothing when vmimage source is upload, dataVolume will be created by upload handler
		return vmImg, nil
	case harvesterv1.VirtualMachineImageSourceTypeExportVolume:
		return b.initializeExportFromVolume(vmImg)
	default:
		return vmImg, fmt.Errorf("unsupported source type: %s", vmImg.Spec.SourceType)
	}
}

func (b *Backend) Check(vmImg *harvesterv1.VirtualMachineImage) error {
	targetDVNs := b.vmio.GetNamespace(vmImg)
	targetDVName := b.vmio.GetName(vmImg)
	targetDV, err := b.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("DataVolume %s/%s not found, waiting for the initialization", targetDVNs, targetDVName)
			return err
		}
		return fmt.Errorf("failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}

	// upload source type will update the progress on the upload handler
	if vmImg.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeDownload ||
		vmImg.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeExportVolume {
		progress := string(targetDV.Status.Progress)
		if progress != "N/A" && progress != "" {
			// progress format looks like "88.82%", we just need the integer part
			parsedInt := strings.Split(progress, ".")[0]
			progressInt, err := strconv.Atoi(parsedInt)
			if err != nil {
				return fmt.Errorf("failed to convert progress to int: %v", err)
			}
			logrus.Infof("Update CDI DataVolume %s/%s progress: %v", targetDVNs, targetDVName, progressInt)
			_, err = b.vmio.Importing(vmImg, "Image Importing", progressInt)
			if err != nil {
				// just log, we don't care the temporary error here
				logrus.Errorf("failed to update VM Image progress: %v", err)
			}
		}
		logrus.Infof("CDI DataVolume %s/%s status: %s, progress: %v", targetDVNs, targetDVName, targetDV.Status.Phase, progress)
	}
	if targetDV.Status.Phase != cdiv1.Succeeded {
		return common.ErrRetryLater
	}
	if _, err := b.vmio.Imported(vmImg, "", 100, vmImg.Status.Size, vmImg.Status.VirtualSize); err != nil {
		return fmt.Errorf("failed to update VM Image status to Imported: %v", err)
	}

	return nil
}

func (b *Backend) UpdateVirtualSize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	// CDI backend does not need this function, the VirtualSize is updated before the DataVolume is created
	return vmi, nil
}

func (b *Backend) Delete(vmImg *harvesterv1.VirtualMachineImage) error {
	targetDVNs := b.vmio.GetNamespace(vmImg)
	targetDVName := b.vmio.GetName(vmImg)
	_, err := b.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("Target DataVolume (%s/%s) is already gone do no-op found", targetDVNs, targetDVName)
			return nil
		}
		return fmt.Errorf("failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}

	// delete the DataVolume
	if err := b.dataVolumeClient.Delete(targetDVNs, targetDVName, &metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}
	return nil
}

func (b *Backend) AddSidecarHandler() {
}

func (b *Backend) isDataVolumeCreated(vmImg *harvesterv1.VirtualMachineImage) (bool, error) {
	targetDVNs := b.vmio.GetNamespace(vmImg)
	targetDVName := b.vmio.GetName(vmImg)
	_, err := b.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}
	return true, nil
}

func (b *Backend) initializeDownload(vmImg *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	virtualSize, err := fetchImageVirtualSize(b.vmio.GetURL(vmImg))
	if err != nil {
		return vmImg, fmt.Errorf("failed to fetch image virtual size: %v", err)
	}

	size, err := fetchImageSize(b.vmio.GetURL(vmImg))
	if err != nil {
		return vmImg, fmt.Errorf("failed to fetch image size: %v", err)
	}

	// means the image is not qcow format
	if virtualSize == 0 {
		virtualSize = size
	}

	if vmImg.Status.Size == 0 && vmImg.Status.VirtualSize == 0 {
		logrus.Infof("Update VM Image size (%v) and virtual size (%v) before we create the DataVolume", size, virtualSize)
		updatedVMImg, err := b.vmio.UpdateVirtualSizeAndSize(vmImg, virtualSize, size)
		if err != nil {
			return vmImg, fmt.Errorf("failed to update VM Image size and virtual size: %v", err)
		}
		// use latest updated VM Image
		vmImg = updatedVMImg
	}

	dvName := b.vmio.GetName(vmImg)
	dvNamespace := b.vmio.GetNamespace(vmImg)

	targetSC, err := b.scClient.Get(vmImg.Spec.TargetStorageClassName, metav1.GetOptions{})
	if err != nil {
		return vmImg, fmt.Errorf("failed to get StorageClass %s: %v", vmImg.Spec.TargetStorageClassName, err)
	}

	// generate DV source
	dvSource, err := generateDVSource(vmImg, b.vmio.GetSourceType(vmImg))
	if err != nil {
		return vmImg, fmt.Errorf("failed to generate DV source: %v", err)
	}

	// generate DV target storage
	dvTargetStorage, err := generateDVTargetStorage(vmImg)
	if err != nil {
		return vmImg, fmt.Errorf("failed to generate DV target storage: %v", err)
	}
	var boolTrue = true
	dataVolumeTemplate := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: GenerateDVAnnotations(targetSC),
			Name:        dvName,
			Namespace:   dvNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         common.HarvesterAPIV1Beta1,
					Kind:               common.VMImageKind,
					Name:               b.vmio.GetName(vmImg),
					UID:                b.vmio.GetUID(vmImg),
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source:  dvSource,
			Storage: dvTargetStorage,
		},
	}
	if _, err := b.dataVolumeClient.Create(dataVolumeTemplate); err != nil {
		return vmImg, fmt.Errorf("failed to create DataVolume %s/%s: %v", dvNamespace, dvName, err)
	}
	logrus.Infof("DataVolume %s/%s created", dvNamespace, dvName)
	return vmImg, nil
}

func (b *Backend) initializeExportFromVolume(vmImg *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	// export from volume means we will create vm image from the raw volume
	sourcePVC, err := b.pvcCache.Get(b.vmio.GetPVCNamespace(vmImg), b.vmio.GetPVCName(vmImg))
	if err != nil {
		return vmImg, fmt.Errorf("failed to get source PVC %s/%s: %v", vmImg.Spec.PVCNamespace, vmImg.Spec.PVCName, err)
	}
	size := sourcePVC.Status.Capacity.Storage().Value()
	virtualSize := size

	if vmImg.Status.Size == 0 && vmImg.Status.VirtualSize == 0 {
		logrus.Infof("Update VM Image size (%v) and virtual size (%v) before we create the DataVolume", size, virtualSize)
		updatedVMImg, err := b.vmio.UpdateVirtualSizeAndSize(vmImg, virtualSize, size)
		if err != nil {
			return vmImg, fmt.Errorf("failed to update VM Image size and virtual size: %v", err)
		}
		// use latest updated VM Image
		vmImg = updatedVMImg
	}

	dvName := b.vmio.GetName(vmImg)
	dvNamespace := b.vmio.GetNamespace(vmImg)

	targetSC, err := b.scClient.Get(vmImg.Spec.TargetStorageClassName, metav1.GetOptions{})
	if err != nil {
		return vmImg, fmt.Errorf("failed to get StorageClass %s: %v", vmImg.Spec.TargetStorageClassName, err)
	}

	// generate DV source
	dvSource, err := generateDVSource(vmImg, b.vmio.GetSourceType(vmImg))
	if err != nil {
		return vmImg, fmt.Errorf("failed to generate DV source: %v", err)
	}

	// generate DV target storage
	dvTargetStorage, err := generateDVTargetStorage(vmImg)
	if err != nil {
		return vmImg, fmt.Errorf("failed to generate DV target storage: %v", err)
	}
	var boolTrue = true
	dataVolumeTemplate := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: GenerateDVAnnotations(targetSC),
			Name:        dvName,
			Namespace:   dvNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         common.HarvesterAPIV1Beta1,
					Kind:               common.VMImageKind,
					Name:               b.vmio.GetName(vmImg),
					UID:                b.vmio.GetUID(vmImg),
					BlockOwnerDeletion: &boolTrue,
				},
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source:  dvSource,
			Storage: dvTargetStorage,
		},
	}
	if _, err := b.dataVolumeClient.Create(dataVolumeTemplate); err != nil {
		return vmImg, fmt.Errorf("failed to create DataVolume %s/%s: %v", dvNamespace, dvName, err)
	}
	logrus.Infof("DataVolume %s/%s created", dvNamespace, dvName)
	return vmImg, nil
}

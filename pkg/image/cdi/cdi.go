package cdi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
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
	pvcCache         ctlcorev1.PersistentVolumeClaimCache
	vmio             common.VMIOperator
}

func GetBackend(ctx context.Context, dataVolumeClient ctlcdiv1.DataVolumeClient, pvcCache ctlcorev1.PersistentVolumeClaimCache, vmio common.VMIOperator) backend.Backend {
	return &Backend{
		ctx:              ctx,
		dataVolumeClient: dataVolumeClient,
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

	switch vmImg.Spec.SourceType {
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
	targetDVNs := vmImg.ObjectMeta.Namespace
	targetDVName := vmImg.ObjectMeta.Name
	targetDV, err := b.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}
	if apierrors.IsNotFound(err) {
		logrus.Info("DataVolume not found, waiting for the initialization")
		return err
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
			logrus.Infof("Update CDI DataVolume progress: %v", progressInt)
			b.vmio.Importing(vmImg, "Image Importing", progressInt)
		}
		logrus.Infof("CDI DataVolume %s/%s status: %s, progress: %v", targetDVNs, targetDVName, targetDV.Status.Phase, progress)
	}
	if targetDV.Status.Phase != cdiv1.Succeeded {
		return common.ErrRetryLater
	}
	b.vmio.Imported(vmImg, "", 100, vmImg.Status.Size, vmImg.Status.VirtualSize)
	return nil
}

func (b *Backend) UpdateVirtualSize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	// CDI backend does not need this function, the VirtualSize is updated before the DataVolume is created
	return vmi, nil
}

func (b *Backend) Delete(vmImg *harvesterv1.VirtualMachineImage) error {
	targetDVNs := vmImg.ObjectMeta.Namespace
	targetDVName := vmImg.ObjectMeta.Name
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
	targetDVNs := vmImg.ObjectMeta.Namespace
	targetDVName := vmImg.ObjectMeta.Name
	_, err := b.dataVolumeClient.Get(targetDVNs, targetDVName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("failed to get DataVolume %s/%s: %v", targetDVNs, targetDVName, err)
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return true, nil
}

func (b *Backend) initializeDownload(vmImg *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	virtualSize, err := fetchImageVirtualSize(vmImg.Spec.URL)
	if err != nil {
		return vmImg, fmt.Errorf("failed to fetch image virtual size: %v", err)
	}

	size, err := fetchImageSize(vmImg.Spec.URL)
	if err != nil {
		return vmImg, fmt.Errorf("failed to fetch image size: %v", err)
	}

	// means the image is not qcow format
	if virtualSize == 0 {
		virtualSize = size
	}

	if vmImg.Status.Size == 0 && vmImg.Status.VirtualSize == 0 {
		logrus.Infof("Update VM Image size (%v) and virtual size (%v) before we create the DataVolume", size, virtualSize)
		vmImgNew := vmImg.DeepCopy()
		vmImgNew.Status.Size = size
		vmImgNew.Status.VirtualSize = virtualSize
		updatedVMImg, err := b.vmio.UpdateVMI(vmImg, vmImgNew)
		if err != nil {
			return vmImg, fmt.Errorf("failed to update VM Image size and virtual size: %v", err)
		}
		// use latest updated VM Image
		vmImg = updatedVMImg
	}

	dvName := vmImg.ObjectMeta.Name
	dvNamespace := vmImg.ObjectMeta.Namespace

	// generate DV source
	dvSource, err := generateDVSource(vmImg)
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
			Name:      dvName,
			Namespace: dvNamespace,
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
	sourcePVC, err := b.pvcCache.Get(vmImg.Spec.PVCNamespace, vmImg.Spec.PVCName)
	if err != nil {
		return vmImg, fmt.Errorf("failed to get source PVC %s/%s: %v", vmImg.Spec.PVCNamespace, vmImg.Spec.PVCName, err)
	}
	size := sourcePVC.Status.Capacity.Storage().Value()
	virtualSize := size

	if vmImg.Status.Size == 0 && vmImg.Status.VirtualSize == 0 {
		logrus.Infof("Update VM Image size (%v) and virtual size (%v) before we create the DataVolume", size, virtualSize)
		vmImgNew := vmImg.DeepCopy()
		vmImgNew.Status.Size = size
		vmImgNew.Status.VirtualSize = virtualSize
		updatedVMImg, err := b.vmio.UpdateVMI(vmImg, vmImgNew)
		if err != nil {
			return vmImg, fmt.Errorf("failed to update VM Image size and virtual size: %v", err)
		}
		// use latest updated VM Image
		vmImg = updatedVMImg
	}

	dvName := vmImg.ObjectMeta.Name
	dvNamespace := vmImg.ObjectMeta.Namespace

	// generate DV source
	dvSource, err := generateDVSource(vmImg)
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
			Name:      dvName,
			Namespace: dvNamespace,
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

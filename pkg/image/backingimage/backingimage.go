package backingimage

import (
	"context"
	errs "errors"
	"fmt"
	"strconv"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhmanager "github.com/longhorn/longhorn-manager/manager"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

const (
	backingImageControllerName = "backing-image-controller"
)

var (
	errBackingImage = errs.New("failed to get backing image")
	errStorageClass = errs.New("failed to get storage class")
)

type Backend struct {
	ctx          context.Context
	scClient     ctlstoragev1.StorageClassClient
	scCache      ctlstoragev1.StorageClassCache
	biController ctllhv1.BackingImageController
	biClient     ctllhv1.BackingImageClient
	biCache      ctllhv1.BackingImageCache
	pvcCache     ctlcorev1.PersistentVolumeClaimCache
	secretCache  ctlcorev1.SecretCache
	vmiClient    ctlharvesterv1.VirtualMachineImageClient
	vmiCache     ctlharvesterv1.VirtualMachineImageCache
	vmio         common.VMIOperator
}

func GetBackend(ctx context.Context, scClient ctlstoragev1.StorageClassClient, scCache ctlstoragev1.StorageClassCache,
	biController ctllhv1.BackingImageController, biClient ctllhv1.BackingImageClient, biCache ctllhv1.BackingImageCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache, secretCache ctlcorev1.SecretCache,
	vmiClient ctlharvesterv1.VirtualMachineImageClient, vmiCache ctlharvesterv1.VirtualMachineImageCache,
	vmio common.VMIOperator) backend.Backend {
	return &Backend{
		ctx:          ctx,
		scClient:     scClient,
		scCache:      scCache,
		biController: biController,
		biClient:     biClient,
		biCache:      biCache,
		pvcCache:     pvcCache,
		secretCache:  secretCache,
		vmiClient:    vmiClient,
		vmiCache:     vmiCache,
		vmio:         vmio,
	}
}

func (bib *Backend) deleteBackingImage(vmi *harvesterv1.VirtualMachineImage) error {
	biName, err := util.GetBackingImageName(bib.biCache, vmi)
	if err != nil {
		return err
	}

	propagation := metav1.DeletePropagationForeground
	return bib.biClient.Delete(util.LonghornSystemNamespaceName, biName, &metav1.DeleteOptions{PropagationPolicy: &propagation})
}

func (bib *Backend) deleteStorageClass(vmi *harvesterv1.VirtualMachineImage) error {
	return bib.scClient.Delete(util.GetImageStorageClassName(vmi), &metav1.DeleteOptions{})
}

func (bib *Backend) deleteBackingImageAndStorageClass(vmi *harvesterv1.VirtualMachineImage) error {
	if err := bib.deleteBackingImage(vmi); err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err := bib.deleteStorageClass(vmi); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (bib *Backend) createBackingImage(vmi *harvesterv1.VirtualMachineImage) error {
	if cachedBI, _ := util.GetBackingImage(bib.biCache, vmi); cachedBI != nil && cachedBI.DeletionTimestamp != nil {
		return fmt.Errorf("backing image %s is being deleted", cachedBI.Name)
	}

	biName, err := util.GetBackingImageName(bib.biCache, vmi)
	if err != nil {
		return err
	}

	vmio := bib.vmio
	// use target storage class's numberOfReplicas as minNumberOfCopies for image HA
	numOfCopiesStr := vmio.GetSCParameters(vmi)[longhorntypes.OptionNumberOfReplicas]
	numOfCopies, err := strconv.Atoi(numOfCopiesStr)
	if err != nil {
		return err
	}

	bi := &lhv1beta2.BackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      biName,
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.AnnotationImageID: ref.Construct(vmio.GetNamespace(vmi), vmio.GetName(vmi)),
			},
		},
		Spec: lhv1beta2.BackingImageSpec{
			SourceType:        lhv1beta2.BackingImageDataSourceType(vmio.GetSourceType(vmi)),
			SourceParameters:  map[string]string{},
			Checksum:          vmio.GetChecksum(vmi),
			MinNumberOfCopies: numOfCopies,
		},
	}

	switch vmio.GetSourceType(vmi) {
	case harvesterv1.VirtualMachineImageSourceTypeDownload:
		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeDownloadParameterURL] = vmio.GetURL(vmi)
	case harvesterv1.VirtualMachineImageSourceTypeExportVolume:
		pvc, err := bib.pvcCache.Get(vmio.GetPVCNamespace(vmi), vmio.GetPVCName(vmi))
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", vmio.GetPVCNamespace(vmi), vmio.GetPVCName(vmi), err.Error())
		}

		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeExportFromVolumeParameterVolumeName] = pvc.Spec.VolumeName
		bi.Spec.SourceParameters[lhmanager.DataSourceTypeExportFromVolumeParameterExportType] = lhmanager.DataSourceTypeExportFromVolumeParameterExportTypeRAW
	case harvesterv1.VirtualMachineImageSourceTypeRestore:
		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeRestoreParameterBackupURL] = vmio.GetURL(vmi)
	case harvesterv1.VirtualMachineImageSourceTypeClone:
		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeCloneParameterEncryption] = vmio.GetSecurityCryptoOption(vmi)

		sourceImage, err := bib.vmiClient.Get(vmio.GetSecuritySrcImgNamespace(vmi), vmio.GetSecuritySrcImgName(vmi), metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get source vmimage %s/%s, error: %s", vmio.GetSecuritySrcImgNamespace(vmi), vmio.GetSecuritySrcImgName(vmi), err.Error())
		}

		sourceBiName, err := util.GetBackingImageName(bib.biCache, sourceImage)
		if err != nil {
			return fmt.Errorf("failed to get source backing image name for vmimage %s/%s, error: %s", sourceImage.Namespace, sourceImage.Name, err.Error())
		}

		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeCloneParameterBackingImage] = sourceBiName

		targetImage := vmi

		if vmio.IsDecryptOperation(vmi) {
			// if try to decrypt image, we should get the storage class of source virtual machine image.
			targetImage = sourceImage
		}

		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeCloneParameterSecret] = vmio.GetSCParameters(targetImage)[util.CSINodePublishSecretNameKey]
		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeCloneParameterSecretNamespace] = vmio.GetSCParameters(targetImage)[util.CSINodePublishSecretNamespaceKey]
	}

	_, err = bib.biClient.Create(bi)
	return err
}

func (bib *Backend) createStorageClass(vmi *harvesterv1.VirtualMachineImage) error {
	if cachedSC, _ := bib.scCache.Get(util.GetImageStorageClassName(vmi)); cachedSC != nil && cachedSC.DeletionTimestamp != nil {
		return fmt.Errorf("storage class %s is being deleted", cachedSC.Name)
	}

	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate

	params, err := util.GetImageStorageClassParameters(bib.biCache, vmi)
	if err != nil {
		return err
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.GetImageStorageClassName(vmi),
		},
		Provisioner:          longhorntypes.LonghornDriverName,
		ReclaimPolicy:        &reclaimPolicy,
		AllowVolumeExpansion: pointer.BoolPtr(true),
		VolumeBindingMode:    &volumeBindingMode,
		Parameters:           params,
	}

	_, err = bib.scClient.Create(sc)
	return err
}

func (bib *Backend) createBackingImageAndStorageClass(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := bib.createBackingImage(vmi); err != nil && !errors.IsAlreadyExists(err) {
		return vmi, err
	}
	if err := bib.createStorageClass(vmi); err != nil && !errors.IsAlreadyExists(err) {
		return vmi, err
	}
	return vmi, nil
}

func (bib *Backend) Initialize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := bib.deleteBackingImageAndStorageClass(vmi); err != nil {
		return vmi, err
	}

	checkedImg, err := bib.vmio.CheckURLAndUpdate(vmi)
	if err != nil {
		return checkedImg, err
	}

	toUpdate, err := bib.createBackingImageAndStorageClass(checkedImg)
	if err != nil {
		return toUpdate, err
	}
	return bib.vmio.UpdateVMI(checkedImg, toUpdate)
}

func (bib *Backend) Check(vmi *harvesterv1.VirtualMachineImage) error {
	bi, err := util.GetBackingImage(bib.biCache, vmi)
	if errors.IsNotFound(err) {
		return common.ErrRetryAble
	}

	if err != nil {
		return errBackingImage
	}

	if bi.DeletionTimestamp != nil {
		return common.ErrRetryLater
	}

	for _, status := range bi.Status.DiskFileStatusMap {
		if status.State == lhv1beta2.BackingImageStateFailed {
			return common.ErrRetryAble
		}
	}

	sc, err := bib.scCache.Get(util.GetImageStorageClassName(vmi))
	if errors.IsNotFound(err) {
		return err
	}

	if err != nil {
		return errStorageClass
	}

	if sc != nil && sc.DeletionTimestamp != nil {
		return common.ErrRetryLater
	}

	return nil
}

func (bib *Backend) UpdateVirtualSize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	bi, err := util.GetBackingImage(bib.biCache, vmi)
	if err != nil {
		return vmi, err
	}

	return bib.vmio.UpdateVirtualSize(vmi, bi.Status.VirtualSize)
}

func (bib *Backend) deleteVMImageMetadata(vmi *harvesterv1.VirtualMachineImage) error {
	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return err
	}

	vmio := bib.vmio
	// when backup target has been reset to default, skip following
	if target.IsDefaultBackupTarget() {
		logrus.WithFields(logrus.Fields{
			"namespace": vmio.GetNamespace(vmi),
			"name":      vmio.GetName(vmi),
		}).Debugf("skip deleting vm image metadata, because backup target setting is default")
		return nil
	}

	if !backuputil.IsBackupTargetSame(vmi.Status.BackupTarget, target) {
		logrus.WithFields(logrus.Fields{
			"namespace":            vmio.GetNamespace(vmi),
			"name":                 vmio.GetName(vmi),
			"status.backupTarget":  vmio.GetBackupTarget(vmi),
			"setting.backupTarget": target,
		}).Debugf("skip deleting vm image metadata, because status backup target is different from backup target setting")
		return nil
	}

	bsDriver, err := backuputil.GetBackupStoreDriver(bib.secretCache, target)
	if err != nil {
		return err
	}

	destURL := backuputil.GetVMImageMetadataFilePath(vmio.GetNamespace(vmi), vmio.GetName(vmi))
	if exist := bsDriver.FileExists(destURL); exist {
		logrus.WithFields(logrus.Fields{
			"namespace":    vmio.GetNamespace(vmi),
			"name":         vmio.GetName(vmi),
			"backupTarget": vmio.GetBackupTarget(vmi),
		}).Debugf("delete vm image metadata in backup target")
		return bsDriver.Remove(destURL)
	}
	return nil
}

func (bib *Backend) Delete(vmi *harvesterv1.VirtualMachineImage) error {
	if err := bib.deleteBackingImageAndStorageClass(vmi); err != nil {
		return err
	}

	return bib.deleteVMImageMetadata(vmi)
}

func (bib *Backend) AddSidecarHandler() {
	backingImageHandler := &backingImageHandler{
		vmiCache: bib.vmiCache,
		vmio:     bib.vmio,
	}

	bib.biController.OnChange(bib.ctx, backingImageControllerName, backingImageHandler.OnChanged)
}

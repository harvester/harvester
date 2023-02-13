package image

import (
	"fmt"
	"net/http"
	"time"

	lhcontroller "github.com/longhorn/longhorn-manager/controller"
	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhmanager "github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/rancher/norman/condition"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	lhv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

// vmImageHandler syncs status on vm image changes, and manage a storageclass & a backingimage per vm image
type vmImageHandler struct {
	httpClient        http.Client
	storageClasses    ctlstoragev1.StorageClassClient
	images            ctlharvesterv1.VirtualMachineImageClient
	imageController   ctlharvesterv1.VirtualMachineImageController
	backingImages     lhv1beta1.BackingImageClient
	backingImageCache lhv1beta1.BackingImageCache
	pvcCache          ctlcorev1.PersistentVolumeClaimCache
}

func (h *vmImageHandler) OnChanged(_ string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil || image.DeletionTimestamp != nil || harvesterv1.ImageRetryLimitExceeded.IsTrue(image) {
		return image, nil
	}

	if harvesterv1.ImageImported.IsTrue(image) {
		// sync display_name to labels in order to list by labelSelector
		if image.Spec.DisplayName != image.Labels[util.LabelImageDisplayName] {
			toUpdate := image.DeepCopy()
			if toUpdate.Labels == nil {
				toUpdate.Labels = map[string]string{}
			}
			toUpdate.Labels[util.LabelImageDisplayName] = image.Spec.DisplayName
			return h.images.Update(toUpdate)
		}

		return image, nil
	}

	needRetry := false
	backingImage, err := h.backingImageCache.Get(util.LonghornSystemNamespaceName, util.GetBackingImageName(image))
	if err != nil {
		if !errors.IsNotFound(err) {
			logrus.Errorf("failed to get backing image %s/%s, err: %v", util.LonghornSystemNamespaceName, util.GetBackingImageName(image), err)
			return image, err
		}
		needRetry = true
	} else {
		for _, status := range backingImage.Status.DiskFileStatusMap {
			if status.State == v1beta1.BackingImageStateFailed {
				needRetry = true
				break
			}
		}
	}

	if needRetry {
		if image.Status.LastFailedTime == "" {
			return h.initialize(image)
		}

		return h.handleRetry(image)
	}

	return image, nil
}

func (h *vmImageHandler) OnRemove(_ string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}
	if err := h.deleteBackingImageAndStorageClass(image); err != nil {
		return image, err
	}
	return image, nil
}

func (h *vmImageHandler) initialize(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := h.deleteBackingImageAndStorageClass(image); err != nil {
		return image, err
	}

	image, err := h.checkImage(image)
	if err != nil {
		logrus.Errorf("failed to check image %s/%s, err: %v", image.Namespace, image.Name, err)
		h.imageController.EnqueueAfter(image.Namespace, image.Name, 10*time.Second)
		return h.images.Update(image)
	}
	return h.createBackingImageAndStorageClass(image)
}

func (h *vmImageHandler) checkImage(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image.Spec.SourceType != harvesterv1.VirtualMachineImageSourceTypeDownload {
		return image, nil
	}

	toUpdate := image.DeepCopy()
	toUpdate.Status.AppliedURL = image.Spec.URL
	resp, err := h.httpClient.Head(image.Spec.URL)
	if err != nil {
		toUpdate = handleFail(toUpdate, condition.Cond(harvesterv1.ImageInitialized), err)
		return toUpdate, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		err = fmt.Errorf("got %d status code from %s", resp.StatusCode, image.Spec.URL)
		toUpdate = handleFail(toUpdate, condition.Cond(harvesterv1.ImageInitialized), err)
		return toUpdate, err
	}

	if resp.ContentLength > 0 {
		toUpdate.Status.Size = resp.ContentLength
	}
	return toUpdate, nil
}

func (h *vmImageHandler) createBackingImageAndStorageClass(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := h.createBackingImage(image); err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}
	if err := h.createStorageClass(image); err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	toUpdate := image.DeepCopy()
	toUpdate.Status.AppliedURL = toUpdate.Spec.URL
	toUpdate.Status.StorageClassName = util.GetImageStorageClassName(image.Name)
	toUpdate.Status.Progress = 0

	harvesterv1.ImageImported.Unknown(toUpdate)
	harvesterv1.ImageImported.Reason(toUpdate, "Importing")
	harvesterv1.ImageImported.LastUpdated(toUpdate, time.Now().Format(time.RFC3339))
	harvesterv1.ImageInitialized.True(toUpdate)
	harvesterv1.ImageInitialized.Message(toUpdate, "")
	harvesterv1.ImageInitialized.Reason(toUpdate, "Initialized")
	harvesterv1.ImageInitialized.LastUpdated(toUpdate, time.Now().Format(time.RFC3339))

	return h.images.Update(toUpdate)
}

func (h *vmImageHandler) createBackingImage(image *harvesterv1.VirtualMachineImage) error {
	bi := &v1beta1.BackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.GetBackingImageName(image),
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.AnnotationImageID: ref.Construct(image.Namespace, image.Name),
			},
		},
		Spec: v1beta1.BackingImageSpec{
			SourceType:       v1beta1.BackingImageDataSourceType(image.Spec.SourceType),
			SourceParameters: map[string]string{},
			Checksum:         image.Spec.Checksum,
		},
	}
	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeDownload {
		bi.Spec.SourceParameters[v1beta1.DataSourceTypeDownloadParameterURL] = image.Spec.URL
	}

	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeExportVolume {
		pvc, err := h.pvcCache.Get(image.Spec.PVCNamespace, image.Spec.PVCName)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", image.Spec.PVCName, image.Namespace, err.Error())
		}

		bi.Spec.SourceParameters[lhcontroller.DataSourceTypeExportFromVolumeParameterVolumeName] = pvc.Spec.VolumeName
		bi.Spec.SourceParameters[lhmanager.DataSourceTypeExportFromVolumeParameterExportType] = lhmanager.DataSourceTypeExportFromVolumeParameterExportTypeRAW
	}

	_, err := h.backingImages.Create(bi)
	return err
}

func (h *vmImageHandler) createStorageClass(image *harvesterv1.VirtualMachineImage) error {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.GetImageStorageClassName(image.Name),
		},
		Provisioner:          types.LonghornDriverName,
		ReclaimPolicy:        &reclaimPolicy,
		AllowVolumeExpansion: pointer.BoolPtr(true),
		VolumeBindingMode:    &volumeBindingMode,
		Parameters:           util.GetImageStorageClassParameters(image),
	}

	_, err := h.storageClasses.Create(sc)
	return err
}

func (h *vmImageHandler) deleteBackingImage(image *harvesterv1.VirtualMachineImage) error {
	return h.backingImages.Delete(util.LonghornSystemNamespaceName, util.GetBackingImageName(image), &metav1.DeleteOptions{})
}

func (h *vmImageHandler) deleteStorageClass(image *harvesterv1.VirtualMachineImage) error {
	return h.storageClasses.Delete(util.GetImageStorageClassName(image.Name), &metav1.DeleteOptions{})
}

func (h *vmImageHandler) deleteBackingImageAndStorageClass(image *harvesterv1.VirtualMachineImage) error {
	if err := h.deleteBackingImage(image); err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err := h.deleteStorageClass(image); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (h *vmImageHandler) handleRetry(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	ts, err := time.Parse(time.RFC3339, image.Status.LastFailedTime)
	if err != nil {
		logrus.Errorf("failed to parse lastFailedTime %s in image %s/%s, err: %v", image.Status.LastFailedTime, image.Namespace, image.Name, err)
		toUpdate := image.DeepCopy()
		toUpdate.Status.LastFailedTime = time.Now().Format(time.RFC3339)
		return h.images.Update(toUpdate)
	}

	ts = ts.Add(10 * time.Second)
	if time.Now().Before(ts) {
		h.imageController.EnqueueAfter(image.Namespace, image.Name, 10*time.Second)
		return image, nil
	}
	return h.initialize(image)
}

func handleFail(image *harvesterv1.VirtualMachineImage, cond condition.Cond, err error) *harvesterv1.VirtualMachineImage {
	image.Status.Failed++
	image.Status.LastFailedTime = time.Now().Format(time.RFC3339)
	if image.Status.Failed > image.Spec.Retry {
		harvesterv1.ImageRetryLimitExceeded.True(image)
		harvesterv1.ImageRetryLimitExceeded.Message(image, "VMImage has reached the specified retry limit")
		cond.False(image)
		cond.Message(image, err.Error())
	}
	return image
}

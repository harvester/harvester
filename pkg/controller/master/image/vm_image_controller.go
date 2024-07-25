package image

import (
	"fmt"
	"net/http"
	"time"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhmanager "github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/rancher/norman/condition"
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
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

const (
	checkInterval = 1 * time.Second
)

// vmImageHandler syncs status on vm image changes, and manage a storageclass & a backingimage per vm image
type vmImageHandler struct {
	httpClient        http.Client
	storageClasses    ctlstoragev1.StorageClassClient
	storageClassCache ctlstoragev1.StorageClassCache
	images            ctlharvesterv1.VirtualMachineImageClient
	imageController   ctlharvesterv1.VirtualMachineImageController
	backingImages     ctllhv1.BackingImageClient
	backingImageCache ctllhv1.BackingImageCache
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

		// sync virtualSize (handles the case for existing images that were
		// imported before this field was added, because adding the field to
		// the CRD triggers vmImageHandler.OnChanged)
		if image.Status.VirtualSize == 0 {
			toUpdate := image.DeepCopy()
			bi, err := util.GetBackingImage(h.backingImageCache, toUpdate)
			if err != nil {
				// If for some reason we're unable to get the backing image,
				// we set the BackingImageMissing condition on the image,
				// including the error message, so it can be seen via
				// `kubectl describe`.  The image is not marked failed or
				// anything, in case this is some sort of transient error.
				// In the unlikely event that we do hit this case, and whatever
				// error was present is later fixed, the user can re-trigger
				// this code by making a temporary change to the image (e.g.
				// add/change the image description).  Once set, the
				// BackingImageMissing condition is never automatically removed,
				// but can be deleted manually with `kubectl edit`.
				logrus.WithError(err).WithFields(logrus.Fields{
					"namespace": toUpdate.Namespace,
					"name":      toUpdate.Name,
				}).Error("failed to get backing image for vmimage")
				harvesterv1.BackingImageMissing.True(toUpdate)
				harvesterv1.BackingImageMissing.Message(toUpdate,
					fmt.Sprintf("Failed to get backing image: %s", err.Error()))
			} else {
				toUpdate.Status.VirtualSize = bi.Status.VirtualSize
			}
			return h.images.Update(toUpdate)
		}

		return image, nil
	}

	return h.processVMImage(image)
}

func (h *vmImageHandler) processVMImage(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	bi, err := util.GetBackingImage(h.backingImageCache, image)
	if err != nil {
		if !errors.IsNotFound(err) {
			logrus.WithError(err).WithFields(logrus.Fields{
				"namespace": image.Namespace,
				"name":      image.Name,
			}).Error("failed to get backing image for vmimage")
			return image, err
		}
		return h.handleRetry(image)
	}

	if bi.DeletionTimestamp != nil {
		h.imageController.EnqueueAfter(image.Namespace, image.Name, checkInterval)
		return image, nil
	}

	for _, status := range bi.Status.DiskFileStatusMap {
		if status.State == lhv1beta2.BackingImageStateFailed {
			return h.handleRetry(image)
		}
	}

	sc, scErr := h.storageClassCache.Get(util.GetImageStorageClassName(image.Name))
	if scErr != nil {
		if !errors.IsNotFound(scErr) {
			logrus.WithError(scErr).WithFields(logrus.Fields{
				"namespace": image.Namespace,
				"name":      image.Name,
			}).Error("failed to get storage class for vmimage")
			return image, scErr
		}
		return h.handleRetry(image)
	}

	if sc != nil && sc.DeletionTimestamp != nil {
		h.imageController.EnqueueAfter(image.Namespace, image.Name, checkInterval)
		return image, nil
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

	toUpdate := image.DeepCopy()
	toUpdate, err := h.checkImage(toUpdate)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": toUpdate.Namespace,
			"name":      toUpdate.Name,
		}).Error("failed to check vmimage")
		return h.images.Update(toUpdate)
	}

	return h.createBackingImageAndStorageClass(toUpdate)
}

func (h *vmImageHandler) checkImage(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image.Spec.SourceType != harvesterv1.VirtualMachineImageSourceTypeDownload {
		return image, nil
	}

	image.Status.AppliedURL = image.Spec.URL
	resp, err := h.httpClient.Head(image.Spec.URL)
	if err != nil {
		image = handleFail(image, condition.Cond(harvesterv1.ImageInitialized), err)
		return image, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		err = fmt.Errorf("got %d status code from %s", resp.StatusCode, image.Spec.URL)
		image = handleFail(image, condition.Cond(harvesterv1.ImageInitialized), err)
		return image, err
	}

	if resp.ContentLength > 0 {
		image.Status.Size = resp.ContentLength
	}
	return image, nil
}

func (h *vmImageHandler) createBackingImageAndStorageClass(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if err := h.createBackingImage(image); err != nil && !errors.IsAlreadyExists(err) {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": image.Namespace,
			"name":      image.Name,
		}).Error("failed to create backing image for vmimage")
		toUpdate := handleFail(image, condition.Cond(harvesterv1.ImageInitialized), err)
		return h.images.Update(toUpdate)
	}
	if err := h.createStorageClass(image); err != nil && !errors.IsAlreadyExists(err) {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": image.Namespace,
			"name":      image.Name,
		}).Error("failed to create storage class for vmimage")
		toUpdate := handleFail(image, condition.Cond(harvesterv1.ImageInitialized), err)
		return h.images.Update(toUpdate)
	}

	toUpdate := image.DeepCopy()
	toUpdate.Status.AppliedURL = image.Spec.URL
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
	if cachedBI, _ := util.GetBackingImage(h.backingImageCache, image); cachedBI != nil && cachedBI.DeletionTimestamp != nil {
		return fmt.Errorf("backing image %s is being deleted", cachedBI.Name)
	}

	biName, err := util.GetBackingImageName(h.backingImageCache, image)
	if err != nil {
		return err
	}

	bi := &lhv1beta2.BackingImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      biName,
			Namespace: util.LonghornSystemNamespaceName,
			Annotations: map[string]string{
				util.AnnotationImageID: ref.Construct(image.Namespace, image.Name),
			},
		},
		Spec: lhv1beta2.BackingImageSpec{
			SourceType:       lhv1beta2.BackingImageDataSourceType(image.Spec.SourceType),
			SourceParameters: map[string]string{},
			Checksum:         image.Spec.Checksum,
		},
	}
	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeDownload {
		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeDownloadParameterURL] = image.Spec.URL
	}

	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeExportVolume {
		pvc, err := h.pvcCache.Get(image.Spec.PVCNamespace, image.Spec.PVCName)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", image.Spec.PVCName, image.Namespace, err.Error())
		}

		bi.Spec.SourceParameters[lhv1beta2.DataSourceTypeExportFromVolumeParameterVolumeName] = pvc.Spec.VolumeName
		bi.Spec.SourceParameters[lhmanager.DataSourceTypeExportFromVolumeParameterExportType] = lhmanager.DataSourceTypeExportFromVolumeParameterExportTypeRAW
	}

	_, err = h.backingImages.Create(bi)
	return err
}

func (h *vmImageHandler) createStorageClass(image *harvesterv1.VirtualMachineImage) error {
	if cachedSC, _ := h.storageClassCache.Get(util.GetImageStorageClassName(image.Name)); cachedSC != nil && cachedSC.DeletionTimestamp != nil {
		return fmt.Errorf("storage class %s is being deleted", cachedSC.Name)
	}

	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate

	params, err := util.GetImageStorageClassParameters(h.backingImageCache, image)
	if err != nil {
		return err
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.GetImageStorageClassName(image.Name),
		},
		Provisioner:          types.LonghornDriverName,
		ReclaimPolicy:        &reclaimPolicy,
		AllowVolumeExpansion: pointer.BoolPtr(true),
		VolumeBindingMode:    &volumeBindingMode,
		Parameters:           params,
	}

	_, err = h.storageClasses.Create(sc)
	return err
}

func (h *vmImageHandler) deleteBackingImage(image *harvesterv1.VirtualMachineImage) error {
	biName, err := util.GetBackingImageName(h.backingImageCache, image)
	if err != nil {
		return err
	}

	propagation := metav1.DeletePropagationForeground
	return h.backingImages.Delete(util.LonghornSystemNamespaceName, biName, &metav1.DeleteOptions{PropagationPolicy: &propagation})
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
	if image.Status.LastFailedTime == "" {
		return h.initialize(image)
	}

	ts, err := time.Parse(time.RFC3339, image.Status.LastFailedTime)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": image.Namespace,
			"name":      image.Name,
		}).Errorf("failed to parse lastFailedTime %s for vmimage", image.Status.LastFailedTime)
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

package image

import (
	"fmt"
	"net/http"

	"github.com/longhorn/longhorn-manager/types"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

const (
	optionBackingImageName = "backingImage"
	optionBackingImageURL  = "backingImageURL"
	optionMigratable       = "migratable"
)

// handler syncs status on vm image changes, and manage a storageclass per image
type handler struct {
	httpClient     http.Client
	storageClasses v1.StorageClassClient
	images         ctlharvesterv1.VirtualMachineImageClient
}

func (h *handler) OnChanged(key string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil || image.DeletionTimestamp != nil {
		return image, nil
	}
	if !harvesterv1.ImageInitialized.IsTrue(image) {
		return h.createStorageClassAndUpdateStatus(image)
	} else if image.Spec.URL != image.Status.AppliedURL {
		// URL is changed, recreate the storageclass
		scName := getBackingImageStorageClassName(image.Name)
		if err := h.storageClasses.Delete(scName, &metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return image, err
		}
		return h.createStorageClassAndUpdateStatus(image)
	}
	return image, nil
}

func (h *handler) OnRemove(key string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}
	scName := getBackingImageStorageClassName(image.Name)
	if err := h.storageClasses.Delete(scName, &metav1.DeleteOptions{}); !errors.IsNotFound(err) && err != nil {
		return image, err
	}
	return image, nil
}

func (h *handler) createStorageClassAndUpdateStatus(image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	sc := getBackingImageStorageClass(image)
	if _, err := h.storageClasses.Create(sc); !errors.IsAlreadyExists(err) && err != nil {
		return image, err
	}

	toUpdate := image.DeepCopy()
	toUpdate.Status.AppliedURL = toUpdate.Spec.URL
	toUpdate.Status.StorageClassName = sc.Name
	harvesterv1.ImageInitialized.True(toUpdate)
	harvesterv1.ImageInitialized.Message(toUpdate, "")

	if image.Spec.URL != "" {
		resp, err := h.httpClient.Head(image.Spec.URL)
		if err != nil {
			harvesterv1.ImageInitialized.False(toUpdate)
			harvesterv1.ImageInitialized.Message(toUpdate, err.Error())
			return h.images.Update(toUpdate)
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			harvesterv1.ImageInitialized.False(toUpdate)
			harvesterv1.ImageInitialized.Message(toUpdate, fmt.Sprintf("got %d status code from %s", resp.StatusCode, image.Spec.URL))
			return h.images.Update(toUpdate)
		}

		if resp.ContentLength > 0 {
			toUpdate.Status.Size = resp.ContentLength
		}
	}

	return h.images.Update(toUpdate)
}

func getBackingImageStorageClassName(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}

func getBackingImageStorageClass(image *harvesterv1.VirtualMachineImage) *storagev1.StorageClass {
	recliamPolicy := corev1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: getBackingImageStorageClassName(image.Name),
		},
		Provisioner:          types.LonghornDriverName,
		ReclaimPolicy:        &recliamPolicy,
		AllowVolumeExpansion: pointer.BoolPtr(true),
		VolumeBindingMode:    &volumeBindingMode,
		Parameters: map[string]string{
			types.OptionNumberOfReplicas:    "3",
			types.OptionStaleReplicaTimeout: "30",
			optionMigratable:                "true",
			optionBackingImageName:          image.Name,
			optionBackingImageURL:           image.Spec.URL,
		},
	}
}

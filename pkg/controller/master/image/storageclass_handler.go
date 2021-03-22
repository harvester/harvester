package image

import (
	"fmt"

	"github.com/longhorn/longhorn-manager/types"
	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/storage/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
)

const (
	optionBackingImageName = "backingImage"
	optionBackingImageURL  = "backingImageURL"
	optionMigratable       = "migratable"
)

// backingImageStorageClassHandler generates a storageclass with backingimage params for each imported vm image
// and removes it when vm image is deleted
type backingImageStorageClassHandler struct {
	storageClasses v1.StorageClassClient
}

func (h *backingImageStorageClassHandler) OnChanged(key string, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	if image == nil || image.DeletionTimestamp != nil {
		return image, nil
	}
	if apisv1alpha1.ImageImported.IsTrue(image) && image.Status.DownloadURL != "" {
		recliamPolicy := corev1.PersistentVolumeReclaimDelete
		volumeBindingMode := storagev1.VolumeBindingImmediate
		sc := &storagev1.StorageClass{
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
				optionBackingImageURL:           image.Status.DownloadURL,
			},
		}
		if _, err := h.storageClasses.Create(sc); !errors.IsAlreadyExists(err) && err != nil {
			return image, err
		}
	}
	return image, nil
}

func (h *backingImageStorageClassHandler) OnRemove(key string, image *apisv1alpha1.VirtualMachineImage) (*apisv1alpha1.VirtualMachineImage, error) {
	if image == nil {
		return nil, nil
	}
	scName := getBackingImageStorageClassName(image.Name)
	if err := h.storageClasses.Delete(scName, &metav1.DeleteOptions{}); !errors.IsNotFound(err) && err != nil {
		return image, err
	}
	return image, nil
}

func getBackingImageStorageClassName(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}

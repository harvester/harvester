package util

import (
	"fmt"
	"net/url"
	"strings"

	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	k8svolumehelpers "k8s.io/cloud-provider/volume/helpers"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
)

const backingimagePrefix = "vmi"

func backingImageLegacyName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", image.Namespace, image.Name)
}

func backingImageLegacyNameV2(image *harvesterv1.VirtualMachineImage) string {
	return lhutil.AutoCorrectName(backingImageLegacyName(image), lhdatastore.NameMaximumLength)
}

func backingImageName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", backingimagePrefix, image.UID)
}

func defaultBackingImageName(image *harvesterv1.VirtualMachineImage) string {
	return lhutil.AutoCorrectName(backingImageName(image), lhdatastore.NameMaximumLength)
}

func restoreBackingImageName(image *harvesterv1.VirtualMachineImage) (string, bool) {
	if image.Spec.SourceType != harvesterv1.VirtualMachineImageSourceTypeRestore {
		return "", false
	}

	parsedURL, err := url.Parse(image.Spec.URL)
	if err != nil {
		return "", false
	}

	name := parsedURL.Query().Get(LonghornOptionBackingImageName)
	if name == "" || !lhutil.ValidateName(name) {
		return "", false
	}

	return name, true
}

func GetRestoreSCName(image *harvesterv1.VirtualMachineImage) (string, bool) {
	biName, ok := restoreBackingImageName(image)
	if !ok {
		return "", false
	}

	scSuffix := strings.TrimPrefix(biName, backingimagePrefix+"-")
	return lhutil.AutoCorrectName(fmt.Sprintf("lh-%s", scSuffix), lhdatastore.NameMaximumLength), true
}

func getBackingImageByNames(
	backingImageCache ctllhv1.BackingImageCache,
	names ...string,
) (*lhv1beta2.BackingImage, error) {
	var lastErr error
	for _, name := range names {
		if name == "" {
			continue
		}

		bi, err := backingImageCache.Get(LonghornSystemNamespaceName, name)
		if err == nil || !errors.IsNotFound(err) {
			return bi, err
		}
		lastErr = err
	}

	return nil, lastErr
}

func backingImageNameForCreate(image *harvesterv1.VirtualMachineImage) string {
	if biName, ok := restoreBackingImageName(image); ok {
		return biName
	}
	return defaultBackingImageName(image)
}

func GetBackingImage(
	backingImageCache ctllhv1.BackingImageCache,
	image *harvesterv1.VirtualMachineImage,
) (*lhv1beta2.BackingImage, error) {
	names := []string{
		backingImageLegacyName(image),
		backingImageLegacyNameV2(image),
		defaultBackingImageName(image),
	}
	if biName, ok := restoreBackingImageName(image); ok {
		names = append([]string{biName}, names...)
	}

	return getBackingImageByNames(backingImageCache, names...)
}

func GetBackingImageName(
	backingImageCache ctllhv1.BackingImageCache,
	image *harvesterv1.VirtualMachineImage,
) (string, error) {
	bi, err := GetBackingImage(backingImageCache, image)
	if err == nil {
		return bi.Name, nil
	}

	if !errors.IsNotFound(err) {
		return "", err
	}

	return backingImageNameForCreate(image), nil
}

func GetBackingImageDataSourceName(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (string, error) {
	//In LH design, backingimagedatasource name is identical with backingimage
	return GetBackingImageName(backingImageCache, image)
}

func GetImageStorageClassParameters(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (map[string]string, error) {
	biName, err := GetBackingImageName(backingImageCache, image)
	if err != nil {
		return nil, err
	}

	params := map[string]string{
		LonghornOptionBackingImageName: biName,
	}

	if image.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeClone && image.Spec.SecurityParameters.CryptoOperation == harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt {
		params[LonghornOptionBackingImageDataSourceName] = string(lhv1beta2.BackingImageDataSourceTypeClone)
	}

	for k, v := range image.Spec.StorageClassParameters {
		params[k] = v
	}
	return params, nil
}

func GetImageDefaultStorageClassParameters() map[string]string {
	return map[string]string{
		longhorntypes.OptionNumberOfReplicas:    "3",
		longhorntypes.OptionStaleReplicaTimeout: "30",
		LonghornOptionMigratable:                "true",
	}
}

func GetVMIBackend(vmi *harvesterv1.VirtualMachineImage) harvesterv1.VMIBackend {
	return vmi.Spec.Backend
}

func GetDefaultSC(scCache ctlstoragev1.StorageClassCache) *storagev1.StorageClass {
	scList, err := GetSCWithSelector(scCache, labels.Everything())
	if err != nil {
		logrus.Warnf("failed to list all storage classes: %v", err)
		return nil
	}

	// find the default storage class
	for _, storageClass := range scList {
		if storageClass.Annotations[AnnotationIsDefaultStorageClassName] == "true" {
			return storageClass
		}
	}

	return nil
}

func GetSCWithSelector(scCache ctlstoragev1.StorageClassCache, selector labels.Selector) ([]*storagev1.StorageClass, error) {
	scList, err := scCache.List(selector)
	if err != nil {
		return nil, err
	}

	if len(scList) == 0 {
		return nil, fmt.Errorf("no storage class found with selector %v", selector)
	}

	return scList, nil
}

// GetImageDiskSizeQuantity returns the minimum disk size a volume created
// from the given image must have, i.e. the image's virtual size (or its
// artifact size if larger), rounded up to whole GiB.
func GetImageDiskSizeQuantity(image *harvesterv1.VirtualMachineImage) (*resource.Quantity, error) {
	imgSize := max(image.Status.VirtualSize, image.Status.Size)
	if imgSize <= 0 {
		return resource.NewQuantity(0, resource.BinarySI), nil
	}

	imgSizeRoundUp, err := k8svolumehelpers.RoundUpToGiB(*resource.NewQuantity(imgSize, resource.BinarySI))
	if err != nil {
		return nil, err
	}

	return resource.NewQuantity(imgSizeRoundUp*k8svolumehelpers.GiB, resource.BinarySI), nil
}

// GetPVCSourceImage returns the VirtualMachineImage a PVC was created from,
// if any. It recognizes two cases:
//   - the PVC is itself the golden-image's backing PVC (marked via the
//     goldenImage annotation, name/namespace match the image).
//   - the PVC was created to hold a clone/import of an image (marked via the
//     imageId annotation).
//
// It returns nil, nil if the PVC has no known image source, the annotation
// is malformed, or the referenced image no longer exists.
func GetPVCSourceImage(pvc *corev1.PersistentVolumeClaim, imageCache ctlharvesterv1.VirtualMachineImageCache) (*harvesterv1.VirtualMachineImage, error) {
	if pvc.Annotations[AnnotationGoldenImage] == "true" {
		image, err := imageCache.Get(pvc.Namespace, pvc.Name)
		if err == nil {
			return image, nil
		}
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	imageID, ok := pvc.Annotations[AnnotationImageID]
	if !ok {
		return nil, nil
	}

	namespace, name, ok := SplitNamespacedName(imageID)
	if !ok {
		return nil, nil
	}

	image, err := imageCache.Get(namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return image, nil
}

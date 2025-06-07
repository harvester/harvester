package util

import (
	"fmt"

	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
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

func GetBackingImage(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (*lhv1beta2.BackingImage, error) {
	bi, err := backingImageCache.Get(LonghornSystemNamespaceName, backingImageLegacyName(image))
	if err == nil {
		return bi, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	bi, err = backingImageCache.Get(LonghornSystemNamespaceName, backingImageLegacyNameV2(image))
	if err == nil {
		return bi, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	rectifyName := lhutil.AutoCorrectName(backingImageName(image), lhdatastore.NameMaximumLength)
	return backingImageCache.Get(LonghornSystemNamespaceName, rectifyName)
}

func GetBackingImageName(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (string, error) {
	bi, err := GetBackingImage(backingImageCache, image)
	if err == nil {
		return bi.Name, nil
	}

	if !errors.IsNotFound(err) {
		return "", err
	}

	return lhutil.AutoCorrectName(backingImageName(image), lhdatastore.NameMaximumLength), nil
}

func GetBackingImageDataSourceName(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (string, error) {
	//In LH design, backingimagedatasource name is identical with backingimage
	return GetBackingImageName(backingImageCache, image)
}

func GetImageStorageClassName(image *harvesterv1.VirtualMachineImage) string {
	if image.Spec.Backend == harvesterv1.VMIBackendCDI {
		return image.Spec.TargetStorageClassName
	}
	return fmt.Sprintf("longhorn-%s", image.Name)
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

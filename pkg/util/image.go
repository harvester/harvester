package util

import (
	"fmt"

	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	"k8s.io/apimachinery/pkg/api/errors"

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

func GetBackingImage(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (*v1beta2.BackingImage, error) {
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

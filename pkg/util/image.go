package util

import (
	"fmt"

	lhdatastore "github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	lhutil "github.com/longhorn/longhorn-manager/util"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
)

func backingImageLegacyName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", image.Namespace, image.Name)
}

func GetBackingImage(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (*v1beta2.BackingImage, error) {
	bi, err := backingImageCache.Get(LonghornSystemNamespaceName, backingImageLegacyName(image))
	if err == nil {
		return bi, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	rectifyName := lhutil.AutoCorrectName(backingImageLegacyName(image), lhdatastore.NameMaximumLength)
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

	return lhutil.AutoCorrectName(backingImageLegacyName(image), lhdatastore.NameMaximumLength), nil
}

func GetBackingImageDataSourceName(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (string, error) {
	//In LH design, backingimagedatasource name is identical with backingimage
	return GetBackingImageName(backingImageCache, image)
}

func GetImageStorageClassName(imageName string) string {
	return fmt.Sprintf("longhorn-%s", imageName)
}

func GetImageStorageClassParameters(backingImageCache ctllhv1.BackingImageCache, image *harvesterv1.VirtualMachineImage) (map[string]string, error) {
	biName, err := GetBackingImageName(backingImageCache, image)
	if err != nil {
		return nil, err
	}

	params := map[string]string{
		LonghornOptionBackingImageName: biName,
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

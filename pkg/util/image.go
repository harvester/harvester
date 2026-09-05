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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctllhv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
)

const (
	backingimagePrefix = "vmi"

	imageStorageClassPrefix = "longhorn"
)

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

// GetImageStorageClassName returns the name of the StorageClass that backs a
// VirtualMachineImage.
//
// The name is derived exclusively from the image's namespace and name, both of
// which are part of the object the user submits. This makes the StorageClass
// name predictable before the object exists, so declarative tooling (GitOps,
// Terraform, CAPI templates) can reference it in the same apply that creates
// the image.
//
// StorageClasses are cluster scoped while VirtualMachineImages are namespaced,
// so the namespace has to be part of the name to keep two identically named
// images in different namespaces from colliding (harvester/harvester#5165).
//
// StorageClass names are DNS subdomains, so the 253 character limit applies
// here, not Longhorn's 40 character limit for its own resources.
func GetImageStorageClassName(image *harvesterv1.VirtualMachineImage) string {
	return lhutil.AutoCorrectName(
		fmt.Sprintf("%s-%s-%s", imageStorageClassPrefix, image.Namespace, image.Name),
		validation.DNS1123SubdomainMaxLength,
	)
}

// GetLegacyImageStorageClassName returns the StorageClass name used by
// Harvester v1.7.x and earlier. It is only used to look up StorageClasses that
// already exist; new StorageClasses are never created with this name because it
// is not unique across namespaces.
func GetLegacyImageStorageClassName(image *harvesterv1.VirtualMachineImage) string {
	return fmt.Sprintf("%s-%s", imageStorageClassPrefix, image.Name)
}

// GetUIDImageStorageClassName returns the UID based StorageClass name used by
// Harvester v1.8.x. It is only used to look up StorageClasses that already
// exist; new StorageClasses are never created with this name because the UID is
// not known until the API server has admitted the VirtualMachineImage.
func GetUIDImageStorageClassName(image *harvesterv1.VirtualMachineImage) string {
	return lhutil.AutoCorrectName(
		fmt.Sprintf("lh-%s", image.UID),
		lhdatastore.NameMaximumLength,
	)
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

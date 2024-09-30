package storageclass

import (
	"fmt"
	"strings"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	ctlstoragev1 "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(storageClassCache ctlstoragev1.StorageClassCache) types.Validator {
	return &storageClassValidator{
		storageClassCache: storageClassCache,
	}
}

type storageClassValidator struct {
	types.DefaultValidator
	storageClassCache ctlstoragev1.StorageClassCache
}

func (v *storageClassValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"storageclasses"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   storagev1.SchemeGroupVersion.Group,
		APIVersion: storagev1.SchemeGroupVersion.Version,
		ObjectType: &storagev1.StorageClass{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *storageClassValidator) Create(_ *types.Request, newObj runtime.Object) error {
	err := v.validateSetUniqueDefault(newObj)
	if err != nil {
		return err
	}

	return v.validateDataLocality(newObj)
}

func (v *storageClassValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	return v.validateSetUniqueDefault(newObj)
}

<<<<<<< HEAD
=======
func (v *storageClassValidator) Delete(_ *types.Request, obj runtime.Object) error {
	sc := obj.(*storagev1.StorageClass)
	err := v.validateHarvesterLonghornSC(sc)
	if err != nil {
		return err
	}
	return v.validateVMImageUsage(sc)
}

>>>>>>> 7c00f127 (Deny to delete the harvester-longhorn sc)
// Harvester rejects setting dataLocality as strict-local, because it makes volume non migrateable,
// beside, strict-local volumes only could have one replica, it make Longhorn block node drain
// https://longhorn.io/docs/1.5.3/references/settings/#node-drain-policy
func (v *storageClassValidator) validateDataLocality(newObj runtime.Object) error {
	sc := newObj.(*storagev1.StorageClass)
	dataLocality, find := sc.Parameters[util.LonghornDataLocality]
	if !find {
		return nil
	}

	lhDataLocality := longhorn.DataLocality(dataLocality)

	if lhDataLocality == longhorn.DataLocalityStrictLocal {
		return werror.NewInvalidError("storage class with strict-local data locality is not allowed", "")
	}

	if lhDataLocality != longhorn.DataLocalityDisabled && lhDataLocality != longhorn.DataLocalityBestEffort {
		message := fmt.Sprintf("storage class with invalid data locality %v, valid values: %v",
			lhDataLocality, strings.Join([]string{`"disabled"`, `"best-effort"`}, ", "))
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *storageClassValidator) validateSetUniqueDefault(newObj runtime.Object) error {
	newSC := newObj.(*storagev1.StorageClass)

	// if new storage class haven't storageclass.kubernetes.io/is-default-class annotation or the value is false.
	if v, ok := newSC.Annotations[util.AnnotationIsDefaultStorageClassName]; !ok || v == "false" {
		return nil
	}

	scList, err := v.storageClassCache.List(labels.Everything())
	if err != nil {
		return werror.NewInvalidError(err.Error(), "")
	}

	for _, sc := range scList {
		if sc.Name == newSC.Name {
			continue
		}

		// return set default error
		// when find another have storageclass.kubernetes.io/is-default-class annotation and value is true.
		if v, ok := sc.Annotations[util.AnnotationIsDefaultStorageClassName]; ok && v == "true" {
			return werror.NewInvalidError("default storage class %s already exists, please reset it first", sc.Name)
		}
	}

	return nil
}
<<<<<<< HEAD
=======

func (v *storageClassValidator) validateEncryption(newObj runtime.Object) error {
	newSC := newObj.(*storagev1.StorageClass)

	// Use util.LonghornOptionEncrypted as the key to check if the storage class is encrypted
	value, ok := newSC.Parameters[util.LonghornOptionEncrypted]
	if !ok {
		return nil
	}

	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return werror.NewInvalidError("invalid value for `encrypted`", "spec.parameters")
	}

	if !enabled {
		return nil
	}

	secretName, secretNamespace, missingParams, err := v.findMissingEncryptionParams(newSC)
	if err != nil {
		return err
	}

	if len(missingParams) != 0 {
		return werror.NewInvalidError(fmt.Sprintf("storage class must contain %s", strings.Join(missingParams, ", ")), "spec.parameters")
	}

	if secretName != "" && secretNamespace != "" {
		_, err := v.secretCache.Get(secretNamespace, secretName)
		if err != nil {
			if errors.IsNotFound(err) {
				return werror.NewInvalidError(fmt.Sprintf("secret %s/%s not found", secretNamespace, secretName), "")
			}
			return werror.NewInvalidError(err.Error(), "")
		}
	}

	return nil
}

func (v *storageClassValidator) findMissingEncryptionParams(newSC *storagev1.StorageClass) (string, string, []string, error) {
	var (
		secretName      string
		secretNamespace string
		missingParams   []string
	)

	for _, pair := range pairs {
		name, nameExists := newSC.Parameters[pair[0]]
		namespace, namespaceExists := newSC.Parameters[pair[1]]

		if !nameExists {
			missingParams = append(missingParams, pair[0])
		}

		if !namespaceExists {
			missingParams = append(missingParams, pair[1])
		}

		if !nameExists || !namespaceExists {
			continue
		}

		if secretName == "" && secretNamespace == "" {
			secretName = name
			secretNamespace = namespace
			continue
		}

		if secretName != name || secretNamespace != namespace {
			return "", "", nil, werror.NewInvalidError(fmt.Sprintf("secret names and namespaces in %s and %s are different from others", pair[0], pair[1]), "")
		}
	}

	return secretName, secretNamespace, missingParams, nil
}

func (v *storageClassValidator) validateVMImageUsage(sc *storagev1.StorageClass) error {
	vmimages, err := v.vmimagesCache.GetByIndex(indexeres.ImageByStorageClass, sc.Name)
	if err != nil {
		return err
	}

	usedVMImages := make([]string, 0, len(vmimages))
	for _, vmimage := range vmimages {
		usedVMImages = append(usedVMImages, vmimage.Name)
	}

	if len(usedVMImages) > 0 {
		return werror.NewInvalidError(fmt.Sprintf("storage class %s is used by virtual machine images: %s", sc.Name, usedVMImages), "")
	}

	return nil
}

// The `harvester-longhorn` SC is created from helm chart and monitored by the managedchart, also used by rancher-monitoring.
// It should not be deleted accidentally.
func (v *storageClassValidator) validateHarvesterLonghornSC(sc *storagev1.StorageClass) error {
	if sc.Name == util.StorageClassHarvesterLonghorn && sc.Annotations != nil {
		if sc.Annotations[util.HelmReleaseNamespaceAnnotation] == util.HarvesterSystemNamespaceName && sc.Annotations[util.HelmReleaseNameAnnotation] == util.HarvesterChartReleaseName {
			return werror.NewInvalidError(fmt.Sprintf("storage class %s is reserved by Harvester and can't be deleted", sc.Name), "")
		}
	}

	return nil
}
>>>>>>> 7c00f127 (Deny to delete the harvester-longhorn sc)

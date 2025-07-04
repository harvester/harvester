package storageclass

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdicaps "kubevirt.io/containerized-data-importer/pkg/storagecapabilities"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	errorMessageReservedStorageClass = "storage class %s is reserved by Harvester and can't be deleted"
)

var (
	availableCiphers  = []string{"aes-xts-plain", "aes-xts-plain64", "aes-cbc-plain", "aes-cbc-plain64", "aes-cbc-essiv:sha256"}
	availablePBKDFs   = []string{"argon2i", "argon2id", "pbkdf2"}
	availableHash     = []string{"sha256", "sha384", "sha512"}
	availableKeySizes = []string{"256", "384", "512"}

	pairs = [][2]string{
		{util.CSIProvisionerSecretNameKey, util.CSIProvisionerSecretNamespaceKey},
		{util.CSINodeStageSecretNameKey, util.CSINodeStageSecretNamespaceKey},
		{util.CSINodePublishSecretNameKey, util.CSINodePublishSecretNamespaceKey},
	}

	allCDICloneStrategies = []string{string(cdiv1.CloneStrategyHostAssisted), string(cdiv1.CloneStrategySnapshot), string(cdiv1.CloneStrategyCsiClone)}
)

func NewValidator(
	storageClassCache ctlstoragev1.StorageClassCache,
	secretCache ctlcorev1.SecretCache,
	vmimagesCache ctlharvesterv1.VirtualMachineImageCache,
	volumeSnapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache) types.Validator {
	return &storageClassValidator{
		storageClassCache:        storageClassCache,
		secretCache:              secretCache,
		vmimagesCache:            vmimagesCache,
		volumeSnapshotClassCache: volumeSnapshotClassCache,
	}
}

type storageClassValidator struct {
	types.DefaultValidator
	storageClassCache        ctlstoragev1.StorageClassCache
	secretCache              ctlcorev1.SecretCache
	vmimagesCache            ctlharvesterv1.VirtualMachineImageCache
	volumeSnapshotClassCache ctlsnapshotv1.VolumeSnapshotClassCache
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
			admissionregv1.Delete,
		},
	}
}

func (v *storageClassValidator) Create(_ *types.Request, newObj runtime.Object) error {
	validators := []func(runtime.Object) error{
		v.validateSetUniqueDefault,
		v.validateDataLocality,
		v.validateEncryption,
		v.validateCDIAnnotations,
	}

	for _, validator := range validators {
		if err := validator(newObj); err != nil {
			return err
		}
	}

	return nil
}

func (v *storageClassValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	validators := []func(runtime.Object) error{
		v.validateSetUniqueDefault,
		v.validateCDIAnnotations,
	}

	for _, validator := range validators {
		if err := validator(newObj); err != nil {
			return err
		}
	}

	return nil
}

func (v *storageClassValidator) Delete(_ *types.Request, obj runtime.Object) error {
	sc := obj.(*storagev1.StorageClass)

	validators := []func(*storagev1.StorageClass) error{
		v.validateReservedStorageClass,
		v.validateVMImageUsage,
	}

	for _, validator := range validators {
		if err := validator(sc); err != nil {
			return err
		}
	}

	return nil
}

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
			return werror.NewInvalidError(fmt.Sprintf("default storage class %s already exists, please reset it first before set %s as default", sc.Name, newSC.Name), "")
		}
	}

	return nil
}

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

	err = v.validateEncryptionParams(newSC)
	if err != nil {
		return err
	}

	secretName, secretNamespace := newSC.Parameters[pairs[0][0]], newSC.Parameters[pairs[0][1]]

	secret, err := v.secretCache.Get(secretNamespace, secretName)
	if err != nil {
		if errors.IsNotFound(err) {
			return werror.NewInvalidError(fmt.Sprintf("secret %s/%s not found", secretNamespace, secretName), "")
		}
		return werror.NewInvalidError(err.Error(), "")
	}

	if err := v.validateEncryptionSecret(secret, secretNamespace, secretName); err != nil {
		return err
	}

	return nil
}

func (v *storageClassValidator) validateEncryptionSecret(secret *corev1.Secret, secretNamespace string, secretName string) error {
	invalid := false
	requiredFields := map[string][]string{
		lhtypes.CryptoKeyCipher:   availableCiphers,
		lhtypes.CryptoKeyHash:     availableHash,
		lhtypes.CryptoKeySize:     availableKeySizes,
		lhtypes.CryptoPBKDF:       availablePBKDFs,
		lhtypes.CryptoKeyProvider: {"secret"},
		lhtypes.CryptoKeyValue:    {""},
	}

	for field, availableValue := range requiredFields {
		value, ok := secret.Data[field]
		if !ok {
			return werror.NewInvalidError(fmt.Sprintf("secret %s/%s is not a valid encryption secret, missing field: %s", secretNamespace, secretName, field), "")
		}

		switch field {
		case lhtypes.CryptoKeyProvider:
			if string(value) != availableValue[0] {
				invalid = true
			}
		case lhtypes.CryptoKeyValue:
			if string(value) == availableValue[0] {
				invalid = true
			}
		default:
			if !slices.Contains(availableValue, string(value)) {
				invalid = true
			}
		}

		if invalid {
			return werror.NewInvalidError(fmt.Sprintf("secret %s/%s is not a valid encryption secret, invalid field: %s", secretNamespace, secretName, field), "")
		}
	}

	return nil
}

func (v *storageClassValidator) validateEncryptionParams(newSC *storagev1.StorageClass) error {
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
			return werror.NewInvalidError(fmt.Sprintf("secret names and namespaces in %s and %s are different from others", pair[0], pair[1]), "")
		}
	}

	if len(missingParams) != 0 {
		return werror.NewInvalidError(fmt.Sprintf("storage class must contain %s", strings.Join(missingParams, ", ")), "spec.parameters")
	}

	if secretName == "" || secretNamespace == "" {
		return werror.NewInvalidError("storage class must contain secret name and namespace", "spec.parameters")
	}

	return nil
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

// Protect the SC with AnnotationIsReservedStorageClass as "true".
// The legacy `harvester-longhorn` SC is created from helm chart and monitored by the managedchart, also used by rancher-monitoring.
// It should not be deleted accidentally.
func (v *storageClassValidator) validateReservedStorageClass(sc *storagev1.StorageClass) error {
	isReserved := sc.Annotations[util.AnnotationIsReservedStorageClass]
	if isReserved == "true" {
		return werror.NewInvalidError(fmt.Sprintf(errorMessageReservedStorageClass, sc.Name), "")
	}

	// if set as false, directly return
	if isReserved == "false" {
		return nil
	}

	// Legacy harvester-longhorn may have no AnnotationIsReservedStorageClass, when it is deployed via helm (managedchart is on top of helm) then deny.
	if sc.Name == util.StorageClassHarvesterLonghorn {
		if sc.Annotations[util.HelmReleaseNamespaceAnnotation] == util.HarvesterSystemNamespaceName && sc.Annotations[util.HelmReleaseNameAnnotation] == util.HarvesterChartReleaseName {
			return werror.NewInvalidError(fmt.Sprintf(errorMessageReservedStorageClass, sc.Name), "")
		}
	}

	return nil
}

func (v *storageClassValidator) validateCDIAnnotations(newObj runtime.Object) error {
	sc := newObj.(*storagev1.StorageClass)

	cdiAnnos := extractCDIAnnotations(sc)
	if err := v.validateLonghornV1(sc, cdiAnnos); err != nil {
		return err
	}

	validators := []func(*storagev1.StorageClass, *cdiAnnotations) error{
		v.validateFilesystemOverhead,
		v.validateCloneStrategy,
		v.validateSnapshotClass,
		v.validateVolumeModeAccessModes,
	}

	for _, validator := range validators {
		if err := validator(sc, cdiAnnos); err != nil {
			return err
		}
	}

	return nil
}

type cdiAnnotations struct {
	fsOverhead                  string
	fsOverheadExists            bool
	cloneStrategy               string
	cloneStrategyExists         bool
	snapshotClass               string
	snapshotClassExists         bool
	volumeModeAccessModes       string
	volumeModeAccessModesExists bool
}

func (c *cdiAnnotations) hasAnyAnnotation() bool {
	return c.fsOverheadExists || c.cloneStrategyExists || c.snapshotClassExists || c.volumeModeAccessModesExists
}

func extractCDIAnnotations(sc *storagev1.StorageClass) *cdiAnnotations {
	annotations := &cdiAnnotations{}

	if sc.Annotations != nil {
		annotations.fsOverhead, annotations.fsOverheadExists = sc.Annotations[util.AnnotationCDIFSOverhead]
		annotations.cloneStrategy, annotations.cloneStrategyExists = sc.Annotations[util.AnnotationStorageProfileCloneStrategy]
		annotations.snapshotClass, annotations.snapshotClassExists = sc.Annotations[util.AnnotationStorageProfileSnapshotClass]
		annotations.volumeModeAccessModes, annotations.volumeModeAccessModesExists = sc.Annotations[util.AnnotationStorageProfileVolumeModeAccessModes]
	}

	return annotations
}

func (v *storageClassValidator) validateLonghornV1(sc *storagev1.StorageClass, annotations *cdiAnnotations) error {
	if !annotations.hasAnyAnnotation() {
		return nil
	}

	if sc.Provisioner != util.CSIProvisionerLonghorn {
		return nil
	}

	// Longhorn v1 does not support CDI annotations, so we throw error
	if dataEngine, ok := sc.Parameters["dataEngine"]; ok && dataEngine == string(longhorn.DataEngineTypeV1) {
		return werror.NewInvalidError("longhorn v1 does not support CDI annotations", "")
	}

	return nil
}

func (v *storageClassValidator) validateFilesystemOverhead(_ *storagev1.StorageClass, annotations *cdiAnnotations) error {
	if !annotations.fsOverheadExists {
		return nil
	}

	if !regexp.MustCompile(util.FSOverheadRegex).MatchString(annotations.fsOverhead) {
		return werror.NewInvalidError(
			fmt.Sprintf("invalid filesystem overhead %s in annotation %s, must be in range [0, 1]",
				annotations.fsOverhead, util.AnnotationCDIFSOverhead), "")
	}

	return nil
}

func (v *storageClassValidator) validateCloneStrategy(_ *storagev1.StorageClass, annotations *cdiAnnotations) error {
	if !annotations.cloneStrategyExists {
		return nil
	}

	_, err := util.ToCloneStrategy(annotations.cloneStrategy)
	if err != nil {
		return werror.NewInvalidError(
			fmt.Sprintf("invalid clone strategy %s in annotation %s, valid values: %s",
				annotations.cloneStrategy,
				util.AnnotationStorageProfileCloneStrategy,
				strings.Join(allCDICloneStrategies, ", ")), "")
	}

	return nil
}

func (v *storageClassValidator) validateSnapshotClass(_ *storagev1.StorageClass, annotations *cdiAnnotations) error {
	if annotations.snapshotClassExists {
		snapshotClass := annotations.snapshotClass
		if snapshotClass == "" {
			return werror.NewInvalidError(
				fmt.Sprintf("snapshot class cannot be empty in annotation %s", util.AnnotationStorageProfileSnapshotClass), "")
		}

		// Check if the VolumeSnapshotClass exists
		if _, err := v.volumeSnapshotClassCache.Get(snapshotClass); err != nil {
			if errors.IsNotFound(err) {
				return werror.NewInvalidError(
					fmt.Sprintf("snapshot class %s in annotation %s not found",
						snapshotClass, util.AnnotationStorageProfileSnapshotClass), "")
			}
			return werror.NewInternalError(
				fmt.Sprintf("failed to get snapshot class %s in annotation %s, error: %v",
					snapshotClass, util.AnnotationStorageProfileSnapshotClass, err))
		}
	}

	if annotations.cloneStrategyExists && annotations.cloneStrategy == string(cdiv1.CloneStrategySnapshot) {
		if !annotations.snapshotClassExists {
			return werror.NewInvalidError(
				fmt.Sprintf("snapshot class must be set in annotation %s when clone strategy is %s",
					util.AnnotationStorageProfileSnapshotClass, cdiv1.CloneStrategySnapshot), "")
		}
	}

	return nil
}

func (v *storageClassValidator) validateVolumeModeAccessModes(sc *storagev1.StorageClass, annotations *cdiAnnotations) error {
	if annotations.volumeModeAccessModesExists {
		return v.validateVolumeModeAccessModesAnnotation(annotations.volumeModeAccessModes)
	}

	// Check if annotation is required for this provisioner
	if _, ok := cdicaps.CapabilitiesByProvisionerKey[sc.Provisioner]; !ok {
		return werror.NewInvalidError(
			fmt.Sprintf("missing annotation %s", util.AnnotationStorageProfileVolumeModeAccessModes), "")
	}

	return nil
}

func (v *storageClassValidator) validateVolumeModeAccessModesAnnotation(volumeModeAccessModes string) error {
	_, err := util.ParseVolumeModeAccessModes(volumeModeAccessModes)
	if err != nil {
		return werror.NewInvalidError(
			fmt.Sprintf("invalid volume mode access modes %s in annotation %s, error: %v",
				volumeModeAccessModes,
				util.AnnotationStorageProfileVolumeModeAccessModes,
				err), "")
	}

	return nil
}

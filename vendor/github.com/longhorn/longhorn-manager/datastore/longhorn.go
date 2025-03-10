package datastore

import (
	"context"
	"fmt"
	"math/bits"
	"math/rand"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/csi/crypto"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	// NameMaximumLength restricted the length due to Kubernetes name limitation
	NameMaximumLength = 40
)

var (
	longhornFinalizerKey = longhorn.SchemeGroupVersion.Group

	// VerificationRetryInterval is the wait time for each verification retries
	VerificationRetryInterval = 100 * time.Millisecond
	// VerificationRetryCounts is the number of times to retry for verification
	VerificationRetryCounts = 20
)

func (s *DataStore) UpdateCustomizedSettings(defaultImages map[types.SettingName]string) error {
	defaultSettingCM, err := s.GetConfigMapRO(s.namespace, types.DefaultDefaultSettingConfigMapName)
	if err != nil {
		return err
	}

	customizedDefaultSettings, err := types.GetCustomizedDefaultSettings(defaultSettingCM)
	if err != nil {
		return err
	}

	availableCustomizedDefaultSettings := s.filterCustomizedDefaultSettings(customizedDefaultSettings, defaultSettingCM.ResourceVersion)

	if err := s.applyCustomizedDefaultSettingsToDefinitions(availableCustomizedDefaultSettings); err != nil {
		return err
	}

	if err := s.syncSettingsWithDefaultImages(defaultImages); err != nil {
		return err
	}

	if err := s.createNonExistingSettingCRsWithDefaultSetting(defaultSettingCM.ResourceVersion); err != nil {
		return err
	}

	return s.syncSettingCRsWithCustomizedDefaultSettings(availableCustomizedDefaultSettings, defaultSettingCM.ResourceVersion)
}

func (s *DataStore) createNonExistingSettingCRsWithDefaultSetting(configMapResourceVersion string) error {
	for _, sName := range types.SettingNameList {
		if _, err := s.GetSettingExactRO(sName); err != nil && apierrors.IsNotFound(err) {
			definition, ok := types.GetSettingDefinition(sName)
			if !ok {
				return fmt.Errorf("BUG: setting %v is not defined", sName)
			}

			setting := &longhorn.Setting{
				ObjectMeta: metav1.ObjectMeta{
					Name:        string(sName),
					Annotations: map[string]string{types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey): configMapResourceVersion},
				},
				Value: definition.Default,
			}

			if _, err := s.CreateSetting(setting); err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
		}
	}

	return nil
}

func (s *DataStore) filterCustomizedDefaultSettings(customizedDefaultSettings map[string]string, defaultSettingCMResourceVersion string) map[string]string {
	availableCustomizedDefaultSettings := make(map[string]string)

	for name, value := range customizedDefaultSettings {
		if !s.shouldApplyCustomizedSettingValue(types.SettingName(name), value, defaultSettingCMResourceVersion) {
			continue
		}

		if err := s.ValidateSetting(string(name), value); err != nil {
			logrus.WithError(err).Warnf("Invalid customized default setting %v with value %v, will continue applying other customized settings", name, value)
			continue
		}

		availableCustomizedDefaultSettings[name] = value
	}
	return availableCustomizedDefaultSettings
}

func (s *DataStore) shouldApplyCustomizedSettingValue(name types.SettingName, value string, defaultSettingCMResourceVersion string) bool {
	setting, err := s.GetSettingExactRO(name)
	if err != nil {
		if !ErrorIsNotFound(err) {
			logrus.WithError(err).Errorf("failed to get customized default setting %v value", name)
			return false
		}
		return true
	}

	configMapResourceVersion := ""
	if setting.Annotations != nil {
		configMapResourceVersion = setting.Annotations[types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey)]
	}

	// Also check the setting definition.
	definition, ok := types.GetSettingDefinition(name)
	if !ok {
		logrus.Errorf("customized default setting %v is not defined", name)
		return true
	}

	return (setting.Value != value || configMapResourceVersion != defaultSettingCMResourceVersion || definition.Default != value)
}

func (s *DataStore) syncSettingsWithDefaultImages(defaultImages map[types.SettingName]string) error {
	for name, value := range defaultImages {
		if err := s.createOrUpdateSetting(name, value, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *DataStore) createOrUpdateSetting(name types.SettingName, value, defaultSettingCMResourceVersion string) error {
	setting, err := s.GetSettingExact(name)
	if err != nil {
		if !ErrorIsNotFound(err) {
			return err
		}

		setting = &longhorn.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name:        string(name),
				Annotations: map[string]string{types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey): defaultSettingCMResourceVersion},
			},
			Value: value,
		}

		if _, err = s.CreateSetting(setting); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		return nil
	}

	if setting.Annotations == nil {
		setting.Annotations = map[string]string{}
	}

	if existingSettingCMResourceVersion, isExist := setting.Annotations[types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey)]; isExist {
		if existingSettingCMResourceVersion == defaultSettingCMResourceVersion && setting.Value == value {
			return nil
		}
	}
	setting.Annotations[types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey)] = defaultSettingCMResourceVersion
	setting.Value = value

	_, err = s.UpdateSetting(setting)
	return err
}

func (s *DataStore) applyCustomizedDefaultSettingsToDefinitions(customizedDefaultSettings map[string]string) error {
	for _, sName := range types.SettingNameList {
		definition, ok := types.GetSettingDefinition(sName)
		if !ok {
			return fmt.Errorf("BUG: setting %v is not defined", sName)
		}

		if definition.ReadOnly {
			continue
		}

		if value, ok := customizedDefaultSettings[string(sName)]; ok {
			definition.Default = value
			types.SetSettingDefinition(sName, definition)
		}
	}
	return nil
}

func (s *DataStore) syncSettingCRsWithCustomizedDefaultSettings(customizedDefaultSettings map[string]string, defaultSettingCMResourceVersion string) error {
	for _, sName := range types.SettingNameList {
		definition, ok := types.GetSettingDefinition(sName)
		if !ok {
			return fmt.Errorf("BUG: setting %v is not defined", sName)
		}

		value, exist := customizedDefaultSettings[string(sName)]
		if !exist {
			continue
		}

		if definition.Required && value == "" {
			continue
		}

		if err := s.createOrUpdateSetting(sName, value, defaultSettingCMResourceVersion); err != nil {
			return err
		}
	}

	return nil
}

// CreateSetting create a Longhorn Settings resource for the given setting and
// namespace
func (s *DataStore) CreateSetting(setting *longhorn.Setting) (*longhorn.Setting, error) {
	// GetSetting automatically create default entry, so no need to double check
	return s.lhClient.LonghornV1beta2().Settings(s.namespace).Create(context.TODO(), setting, metav1.CreateOptions{})
}

// UpdateSetting updates the given Longhorn Settings and verifies update
func (s *DataStore) UpdateSetting(setting *longhorn.Setting) (*longhorn.Setting, error) {
	if setting.Annotations == nil {
		setting.Annotations = make(map[string]string)
	}
	setting.Annotations[types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn)] = ""
	obj, err := s.lhClient.LonghornV1beta2().Settings(s.namespace).Update(context.TODO(), setting, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	delete(obj.Annotations, types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn))
	obj, err = s.lhClient.LonghornV1beta2().Settings(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	verifyUpdate(setting.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getSettingRO(name)
	})
	return obj, nil
}

// UpdateSettingStatus updates Longhorn Setting status and verifies update
func (s *DataStore) UpdateSettingStatus(setting *longhorn.Setting) (*longhorn.Setting, error) {
	obj, err := s.lhClient.LonghornV1beta2().Settings(s.namespace).UpdateStatus(context.TODO(), setting, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(setting.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getSettingRO(name)
	})
	return obj, nil
}

// ValidateSetting checks the given setting value types and condition
func (s *DataStore) ValidateSetting(name, value string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to set the setting %v with invalid value %v", name, value)
	}()
	sName := types.SettingName(name)

	if err := types.ValidateSetting(name, value); err != nil {
		return err
	}

	switch sName {
	case types.SettingNamePriorityClass:
		if value != "" {
			if _, err := s.GetPriorityClass(value); err != nil {
				return errors.Wrapf(err, "failed to get priority class %v before modifying priority class setting", value)
			}
		}
	case types.SettingNameGuaranteedInstanceManagerCPU, types.SettingNameV2DataEngineGuaranteedInstanceManagerCPU:
		guaranteedInstanceManagerCPU, err := s.GetSettingWithAutoFillingRO(sName)
		if err != nil {
			return err
		}
		guaranteedInstanceManagerCPU.Value = value
		if err := types.ValidateCPUReservationValues(sName, guaranteedInstanceManagerCPU.Value); err != nil {
			return err
		}
	case types.SettingNameV1DataEngine:
		old, err := s.GetSettingWithAutoFillingRO(types.SettingNameV1DataEngine)
		if err != nil {
			return err
		}

		if old.Value != value {
			dataEngineEnabled, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}

			_, err = s.ValidateV1DataEngineEnabled(dataEngineEnabled)
			if err != nil {
				return err
			}
		}

	case types.SettingNameV2DataEngine:
		old, err := s.GetSettingWithAutoFillingRO(types.SettingNameV2DataEngine)
		if err != nil {
			return err
		}

		if old.Value != value {
			dataEngineEnabled, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}

			_, err = s.ValidateV2DataEngineEnabled(dataEngineEnabled)
			if err != nil {
				return err
			}
		}
	case types.SettingNameV2DataEngineCPUMask:
		if value == "" {
			return errors.Errorf("cannot set %v setting to empty value", name)
		}
		if err := s.ValidateCPUMask(value); err != nil {
			return err
		}
	case types.SettingNameAutoCleanupSystemGeneratedSnapshot:
		disablePurgeValue, err := s.GetSettingAsBool(types.SettingNameDisableSnapshotPurge)
		if err != nil {
			return err
		}
		if value == "true" && disablePurgeValue {
			return errors.Errorf("cannot set %v setting to true when %v setting is true", name, types.SettingNameDisableSnapshotPurge)
		}
	case types.SettingNameDisableSnapshotPurge:
		autoCleanupValue, err := s.GetSettingAsBool(types.SettingNameAutoCleanupSystemGeneratedSnapshot)
		if err != nil {
			return err
		}
		if value == "true" && autoCleanupValue {
			return errors.Errorf("cannot set %v setting to true when %v setting is true", name, types.SettingNameAutoCleanupSystemGeneratedSnapshot)
		}
	case types.SettingNameSnapshotMaxCount:
		v, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		if v < 2 || v > 250 {
			return fmt.Errorf("%s should be between 2 and 250", name)
		}
	case types.SettingNameDefaultLonghornStaticStorageClass:
		definition, ok := types.GetSettingDefinition(types.SettingNameDefaultLonghornStaticStorageClass)
		if !ok {
			return fmt.Errorf("setting %v is not found", types.SettingNameDefaultLonghornStaticStorageClass)
		}

		if value == definition.Default {
			return nil
		}

		_, err := s.GetStorageClassRO(value)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "cannot use a storage class %v that does not exist to set the setting %v", value, types.SettingNameDefaultLonghornStaticStorageClass)
			}
			return errors.Wrapf(err, "failed to get the storage class %v for setting %v", value, types.SettingNameDefaultLonghornStaticStorageClass)
		}
	}
	return nil
}

func (s *DataStore) ValidateV1DataEngineEnabled(dataEngineEnabled bool) (ims []*longhorn.InstanceManager, err error) {
	if !dataEngineEnabled {
		allV1VolumesDetached, _ims, err := s.AreAllEngineInstancesStopped(longhorn.DataEngineTypeV1)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check volume detachment for %v setting update", types.SettingNameV1DataEngine)
		}
		ims = _ims

		if !allV1VolumesDetached {
			return nil, &types.ErrorInvalidState{Reason: fmt.Sprintf("cannot apply %v setting to Longhorn workloads when there are attached v1 volumes", types.SettingNameV1DataEngine)}
		}
	}

	return ims, nil
}

func (s *DataStore) ValidateV2DataEngineEnabled(dataEngineEnabled bool) (ims []*longhorn.InstanceManager, err error) {
	if !dataEngineEnabled {
		allV2VolumesDetached, _ims, err := s.AreAllEngineInstancesStopped(longhorn.DataEngineTypeV2)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check volume detachment for %v setting update", types.SettingNameV2DataEngine)
		}
		ims = _ims

		if !allV2VolumesDetached {
			return nil, &types.ErrorInvalidState{Reason: fmt.Sprintf("cannot apply %v setting to Longhorn workloads when there are attached v2 volumes", types.SettingNameV2DataEngine)}
		}
	}

	// Check if there is enough hugepages-2Mi capacity for all nodes
	hugepageRequestedInMiB, err := s.GetSettingWithAutoFillingRO(types.SettingNameV2DataEngineHugepageLimit)
	if err != nil {
		return nil, err
	}

	{
		hugepageRequested := resource.MustParse(hugepageRequestedInMiB.Value + "Mi")

		_ims, err := s.ListInstanceManagersRO()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list instance managers for %v setting update", types.SettingNameV2DataEngine)
		}

		for _, im := range _ims {
			node, err := s.GetKubernetesNodeRO(im.Spec.NodeID)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "failed to get Kubernetes node %v for %v setting update", im.Spec.NodeID, types.SettingNameV2DataEngine)
				}

				continue
			}

			if val, ok := node.Labels[types.NodeDisableV2DataEngineLabelKey]; ok && val == types.NodeDisableV2DataEngineLabelKeyTrue {
				// V2 data engine is disabled on this node, don't worry about hugepages
				continue
			}

			if dataEngineEnabled {
				capacity, ok := node.Status.Capacity["hugepages-2Mi"]
				if !ok {
					return nil, errors.Errorf("failed to get hugepages-2Mi capacity for node %v", node.Name)
				}

				hugepageCapacity := resource.MustParse(capacity.String())

				if hugepageCapacity.Cmp(hugepageRequested) < 0 {
					return nil, errors.Errorf("not enough hugepages-2Mi capacity for node %v, requested %v, capacity %v", node.Name, hugepageRequested.String(), hugepageCapacity.String())
				}
			}
		}
	}

	return
}

func (s *DataStore) ValidateCPUMask(value string) error {
	// CPU mask must start with 0x
	cpuMaskRegex := regexp.MustCompile(`^0x[1-9a-fA-F][0-9a-fA-F]*$`)
	if !cpuMaskRegex.MatchString(value) {
		return fmt.Errorf("invalid CPU mask: %s", value)
	}

	maskValue, err := strconv.ParseUint(value[2:], 16, 64) // skip 0x prefix
	if err != nil {
		return fmt.Errorf("failed to parse CPU mask: %s", value)
	}

	// Validate the mask value is not larger than the number of available CPUs
	numCPUs := runtime.NumCPU()
	maxCPUMaskValue := (1 << numCPUs) - 1
	if maskValue > uint64(maxCPUMaskValue) {
		return fmt.Errorf("CPU mask exceeds the maximum allowed value %v for the current system: %s", maxCPUMaskValue, value)
	}

	guaranteedInstanceManagerCPU, err := s.GetSettingAsInt(types.SettingNameV2DataEngineGuaranteedInstanceManagerCPU)
	if err != nil {
		return errors.Wrapf(err, "failed to get %v setting for CPU mask validation", types.SettingNameV2DataEngineGuaranteedInstanceManagerCPU)
	}

	numMilliCPUsRequrestedByMaskValue := calculateMilliCPUs(maskValue)
	if numMilliCPUsRequrestedByMaskValue > int(guaranteedInstanceManagerCPU) {
		return fmt.Errorf("number of CPUs (%v) requested by CPU mask (%v) is larger than the %v setting value (%v)",
			numMilliCPUsRequrestedByMaskValue, value, types.SettingNameV2DataEngineGuaranteedInstanceManagerCPU, guaranteedInstanceManagerCPU)
	}

	return nil
}

func calculateMilliCPUs(mask uint64) int {
	// Count the number of set bits in the mask
	setBits := bits.OnesCount64(mask)

	// Each set bit represents 1000 milliCPUs
	numMilliCPUsRequestedByMaskValue := setBits * 1000

	return numMilliCPUsRequestedByMaskValue
}

func (s *DataStore) AreAllRWXVolumesDetached() (bool, error) {
	volumes, err := s.ListVolumesRO()
	if err != nil {
		return false, err
	}
	for _, volume := range volumes {
		if volume.Spec.AccessMode != longhorn.AccessModeReadWriteMany {
			continue
		}

		volumeAttachment, err := s.GetLHVolumeAttachmentByVolumeName(volume.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}

		if volumeAttachment != nil && len(volumeAttachment.Spec.AttachmentTickets) > 0 {
			return false, nil
		}
	}

	return true, nil
}

func (s *DataStore) AreAllEngineInstancesStopped(dataEngine longhorn.DataEngineType) (bool, []*longhorn.InstanceManager, error) {
	var ims []*longhorn.InstanceManager

	nodes, err := s.ListNodes()
	if err != nil {
		return false, ims, err
	}

	for node := range nodes {
		engineInstanceManagers, err := s.ListInstanceManagersBySelectorRO(node, "", longhorn.InstanceManagerTypeEngine, dataEngine)
		if err != nil && !ErrorIsNotFound(err) {
			return false, ims, err
		}

		aioInstanceManagers, err := s.ListInstanceManagersBySelectorRO(node, "", longhorn.InstanceManagerTypeAllInOne, dataEngine)
		if err != nil && !ErrorIsNotFound(err) {
			return false, ims, err
		}

		imMap := types.ConsolidateInstanceManagers(engineInstanceManagers, aioInstanceManagers)
		for _, instanceManager := range imMap {
			if len(instanceManager.Status.InstanceEngines)+len(instanceManager.Status.Instances) > 0 { // nolint: staticcheck
				return false, ims, err
			}

			ims = append(ims, instanceManager)
		}
	}

	return true, ims, err
}

func (s *DataStore) AreAllDisksRemovedByDiskType(diskType longhorn.DiskType) (bool, error) {
	nodes, err := s.ListNodesRO()
	if err != nil {
		return false, err
	}

	for _, node := range nodes {
		for _, disk := range node.Spec.Disks {
			if disk.Type == diskType {
				return false, nil
			}
		}
	}

	return true, nil
}

func (s *DataStore) AreAllVolumesDetachedState() (bool, error) {
	list, err := s.ListVolumesRO()
	if err != nil {
		return false, errors.Wrapf(err, "failed to list volumes")
	}
	for _, v := range list {
		if v.Status.State != longhorn.VolumeStateDetached {
			return false, nil
		}
	}
	return true, nil
}

func (s *DataStore) getSettingRO(name string) (*longhorn.Setting, error) {
	return s.settingLister.Settings(s.namespace).Get(name)
}

func (s *DataStore) GetSettingWithAutoFillingRO(sName types.SettingName) (*longhorn.Setting, error) {
	definition, ok := types.GetSettingDefinition(sName)
	if !ok {
		return nil, fmt.Errorf("setting %v is not supported", sName)
	}

	resultRO, err := s.getSettingRO(string(sName))
	if err != nil {
		if !ErrorIsNotFound(err) {
			return nil, err
		}
		resultRO = &longhorn.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(sName),
			},
			Value: definition.Default,
		}
	}

	return resultRO, nil
}

// GetSettingExact returns the Setting for the given name and namespace
func (s *DataStore) GetSettingExact(sName types.SettingName) (*longhorn.Setting, error) {
	resultRO, err := s.getSettingRO(string(sName))
	if err != nil {
		return nil, err
	}

	return resultRO.DeepCopy(), nil
}

func (s *DataStore) GetSettingExactRO(sName types.SettingName) (*longhorn.Setting, error) {
	resultRO, err := s.getSettingRO(string(sName))
	if err != nil {
		return nil, err
	}

	return resultRO, nil
}

func (s *DataStore) GetSettingApplied(sName types.SettingName) (bool, error) {
	resultRO, err := s.getSettingRO(string(sName))
	if err != nil {
		return false, err
	}
	return resultRO.Status.Applied, nil
}

// GetSetting will automatically fill the non-existing setting if it's a valid
// setting name.
// The function will not return nil for *longhorn.Setting when error is nil
func (s *DataStore) GetSetting(sName types.SettingName) (*longhorn.Setting, error) {
	resultRO, err := s.GetSettingWithAutoFillingRO(sName)
	if err != nil {
		return nil, err
	}
	return resultRO.DeepCopy(), nil
}

// GetSettingValueExisted returns the value of the given setting name.
// Returns error if the setting does not exist or value is empty
func (s *DataStore) GetSettingValueExisted(sName types.SettingName) (string, error) {
	setting, err := s.GetSettingWithAutoFillingRO(sName)
	if err != nil {
		return "", err
	}
	if setting.Value == "" {
		return "", fmt.Errorf("setting %v is empty", sName)
	}
	return setting.Value, nil
}

// ListSettings lists all Settings in the namespace, and fill with default
// values of any missing entry
func (s *DataStore) ListSettings() (map[types.SettingName]*longhorn.Setting, error) {
	itemMap := make(map[types.SettingName]*longhorn.Setting)

	list, err := s.settingLister.Settings(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		settingField := types.SettingName(itemRO.Name)
		// Ignore the items that we don't recognize
		if _, ok := types.GetSettingDefinition(settingField); ok {
			itemMap[settingField] = itemRO.DeepCopy()
		}
	}
	// fill up the missing entries
	for _, sName := range types.SettingNameList {
		if _, ok := itemMap[sName]; !ok {
			definition, exist := types.GetSettingDefinition(sName)
			if exist {
				itemMap[sName] = &longhorn.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: string(sName),
					},
					Value: definition.Default,
				}
			}
		}
	}

	return itemMap, nil
}

// GetAutoBalancedReplicasSetting retrieves the replica auto-balance setting for
// a Longhorn Volume.
//
// This function prioritized the replica auto-balance setting in the Volume spec (`volume.Spec.ReplicaAutoBalance`).
// If not defined or set to `Ignored`, the global setting will be used.
//
// Parameters:
//   - volume: The Longhorn Volume object.
//   - logger: The logger to use.
//
// Returns:
// - longhorn.ReplicaAutoBalance: The auto-balance setting value for the Volume.
func (s *DataStore) GetAutoBalancedReplicasSetting(volume *longhorn.Volume, logger *logrus.Entry) longhorn.ReplicaAutoBalance {
	var setting longhorn.ReplicaAutoBalance

	volumeSetting := volume.Spec.ReplicaAutoBalance
	if volumeSetting != longhorn.ReplicaAutoBalanceIgnored {
		setting = volumeSetting
	}

	var err error
	if setting == "" {
		globalSetting, _ := s.GetSettingValueExisted(types.SettingNameReplicaAutoBalance)

		if globalSetting == string(longhorn.ReplicaAutoBalanceIgnored) {
			globalSetting = string(longhorn.ReplicaAutoBalanceDisabled)
		}

		setting = longhorn.ReplicaAutoBalance(globalSetting)
	}

	err = types.ValidateReplicaAutoBalance(longhorn.ReplicaAutoBalance(setting))
	if err != nil {
		setting = longhorn.ReplicaAutoBalanceDisabled
		logger.WithError(err).Warnf("replica auto-balance is disabled")
	}
	return setting
}

func (s *DataStore) GetEncryptionSecret(secretNamespace, secretName string) (map[string]string, error) {
	secret, err := s.GetSecretRO(secretNamespace, secretName)
	if err != nil {
		return nil, err
	}
	credentialSecret := make(map[string]string)
	if secret.Data == nil {
		return credentialSecret, nil
	}
	credentialSecret[types.CryptoKeyProvider] = string(secret.Data[types.CryptoKeyProvider])
	credentialSecret[types.CryptoKeyValue] = string(secret.Data[types.CryptoKeyValue])
	credentialSecret[types.CryptoKeyCipher] = string(secret.Data[types.CryptoKeyCipher])
	credentialSecret[types.CryptoKeyHash] = string(secret.Data[types.CryptoKeyHash])
	credentialSecret[types.CryptoKeySize] = string(secret.Data[types.CryptoKeySize])
	credentialSecret[types.CryptoPBKDF] = string(secret.Data[types.CryptoPBKDF])
	return credentialSecret, nil
}

// GetCredentialFromSecret gets the Secret of the given name and namespace
// Returns a new credential object or error
func (s *DataStore) GetCredentialFromSecret(secretName string) (map[string]string, error) {
	secret, err := s.GetSecretRO(s.namespace, secretName)
	if err != nil {
		return nil, err
	}
	credentialSecret := make(map[string]string)
	if secret.Data == nil {
		return credentialSecret, nil
	}
	credentialSecret[types.AWSIAMRoleArn] = string(secret.Data[types.AWSIAMRoleArn])
	credentialSecret[types.AWSAccessKey] = string(secret.Data[types.AWSAccessKey])
	credentialSecret[types.AWSSecretKey] = string(secret.Data[types.AWSSecretKey])
	credentialSecret[types.AWSEndPoint] = string(secret.Data[types.AWSEndPoint])
	credentialSecret[types.AWSCert] = string(secret.Data[types.AWSCert])
	credentialSecret[types.CIFSUsername] = string(secret.Data[types.CIFSUsername])
	credentialSecret[types.CIFSPassword] = string(secret.Data[types.CIFSPassword])
	credentialSecret[types.AZBlobAccountName] = string(secret.Data[types.AZBlobAccountName])
	credentialSecret[types.AZBlobAccountKey] = string(secret.Data[types.AZBlobAccountKey])
	credentialSecret[types.AZBlobEndpoint] = string(secret.Data[types.AZBlobEndpoint])
	credentialSecret[types.AZBlobCert] = string(secret.Data[types.AZBlobCert])
	credentialSecret[types.HTTPSProxy] = string(secret.Data[types.HTTPSProxy])
	credentialSecret[types.HTTPProxy] = string(secret.Data[types.HTTPProxy])
	credentialSecret[types.NOProxy] = string(secret.Data[types.NOProxy])
	credentialSecret[types.VirtualHostedStyle] = string(secret.Data[types.VirtualHostedStyle])
	return credentialSecret, nil
}

func CheckVolume(v *longhorn.Volume) error {
	size, err := util.ConvertSize(v.Spec.Size)
	if err != nil {
		return err
	}
	if v.Name == "" || size == 0 || v.Spec.NumberOfReplicas == 0 {
		return fmt.Errorf("BUG: missing required field %+v", v)
	}

	if v.Spec.Encrypted && size <= crypto.Luks2MinimalVolumeSize {
		return fmt.Errorf("invalid volume size %v, need to be bigger than 16MiB, default LUKS2 header size for encryption", v.Spec.Size)
	}

	errs := validation.IsDNS1123Label(v.Name)
	if len(errs) != 0 {
		return fmt.Errorf("invalid volume name: %+v", errs)
	}
	if len(v.Name) > NameMaximumLength {
		return fmt.Errorf("volume name is too long %v, must be less than %v characters",
			v.Name, NameMaximumLength)
	}
	return nil
}

func getVolumeSelector(volumeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetVolumeLabels(volumeName),
	})
}

// GetOwnerReferencesForVolume returns a list contains single OwnerReference for the
// given volume UID and name
func GetOwnerReferencesForVolume(v *longhorn.Volume) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindVolume,
			UID:        v.UID,
			Name:       v.Name,
		},
	}
}

func GetObjectReferencesForVolume(v *longhorn.Volume) corev1.ObjectReference {
	return corev1.ObjectReference{
		APIVersion: longhorn.SchemeGroupVersion.String(),
		Kind:       types.LonghornKindVolume,
		Name:       v.Name,
		UID:        v.UID,
	}
}

// GetOwnerReferencesForRecurringJob returns a list contains single OwnerReference for the
// given recurringJob name
func GetOwnerReferencesForRecurringJob(recurringJob *longhorn.RecurringJob) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindRecurringJob,
			UID:        recurringJob.UID,
			Name:       recurringJob.Name,
		},
	}
}

// GetOwnerReferencesForBackupTarget returns a list contains single OwnerReference for the
// given backup target name
func GetOwnerReferencesForBackupTarget(backupTarget *longhorn.BackupTarget) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindBackupTarget,
			UID:        backupTarget.UID,
			Name:       backupTarget.Name,
		},
	}
}

// GetOwnerReferencesForBackupVolume returns a list contains single OwnerReference for the
// given backup volume name
func GetOwnerReferencesForBackupVolume(backupVolume *longhorn.BackupVolume) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindBackupVolume,
			UID:        backupVolume.UID,
			Name:       backupVolume.Name,
		},
	}
}

// CreateVolume creates a Longhorn Volume resource and verifies creation
func (s *DataStore) CreateVolume(v *longhorn.Volume) (*longhorn.Volume, error) {
	if err := FixupRecurringJob(v); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().Volumes(s.namespace).Create(context.TODO(), v, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "volume", func(name string) (k8sruntime.Object, error) {
		return s.GetVolumeRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Volume)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for volume")
	}

	return ret.DeepCopy(), nil
}

// UpdateVolume updates Longhorn Volume and verifies update
func (s *DataStore) UpdateVolume(v *longhorn.Volume) (*longhorn.Volume, error) {
	if err := FixupRecurringJob(v); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Volumes(s.namespace).Update(context.TODO(), v, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(v.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetVolumeRO(name)
	})
	return obj, nil
}

// UpdateVolumeStatus updates Longhorn Volume status and verifies update
func (s *DataStore) UpdateVolumeStatus(v *longhorn.Volume) (*longhorn.Volume, error) {
	obj, err := s.lhClient.LonghornV1beta2().Volumes(s.namespace).UpdateStatus(context.TODO(), v, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(v.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetVolumeRO(name)
	})
	return obj, nil
}

// DeleteVolume won't result in immediately deletion since finalizer was set by
// default
func (s *DataStore) DeleteVolume(name string) error {
	return s.lhClient.LonghornV1beta2().Volumes(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForVolume will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForVolume(obj *longhorn.Volume) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Volumes(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for volume %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetVolumeRO(name string) (*longhorn.Volume, error) {
	return s.volumeLister.Volumes(s.namespace).Get(name)
}

// GetVolume returns a new volume object for the given namespace and name
func (s *DataStore) GetVolume(name string) (*longhorn.Volume, error) {
	resultRO, err := s.volumeLister.Volumes(s.namespace).Get(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListVolumesRO returns a list of all Volumes for the given namespace
func (s *DataStore) ListVolumesRO() ([]*longhorn.Volume, error) {
	return s.volumeLister.Volumes(s.namespace).List(labels.Everything())
}

// ListVolumesROWithBackupVolumeName returns a single object contains all volumes
// with the given backup volume name
func (s *DataStore) ListVolumesROWithBackupVolumeName(backupVolumeName string) ([]*longhorn.Volume, error) {
	selector, err := getBackupVolumeSelector(backupVolumeName)
	if err != nil {
		return nil, err
	}
	return s.volumeLister.Volumes(s.namespace).List(selector)
}

// ListVolumesBySelectorRO returns a list of all Volumes for the given namespace
func (s *DataStore) ListVolumesBySelectorRO(selector labels.Selector) ([]*longhorn.Volume, error) {
	return s.volumeLister.Volumes(s.namespace).List(selector)
}

// ListVolumes returns an object contains all Volume
func (s *DataStore) ListVolumes() (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	list, err := s.ListVolumesRO()
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) IsRegularRWXVolume(volumeName string) (bool, error) {
	v, err := s.GetVolumeRO(volumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return v.Spec.AccessMode == longhorn.AccessModeReadWriteMany && !v.Spec.Migratable, nil
}

func MarshalLabelToVolumeRecurringJob(labels map[string]string) map[string]*longhorn.VolumeRecurringJob {
	groupPrefix := fmt.Sprintf(types.LonghornLabelRecurringJobKeyPrefixFmt, types.LonghornLabelRecurringJobGroup) + "/"
	jobPrefix := fmt.Sprintf(types.LonghornLabelRecurringJobKeyPrefixFmt, types.LonghornLabelRecurringJob) + "/"
	jobMapVolumeJob := make(map[string]*longhorn.VolumeRecurringJob)
	for label := range labels {
		if strings.HasPrefix(label, groupPrefix) {
			jobName := strings.TrimPrefix(label, groupPrefix)
			jobMapVolumeJob[jobName] = &longhorn.VolumeRecurringJob{
				Name:    jobName,
				IsGroup: true,
			}
			continue
		} else if strings.HasPrefix(label, jobPrefix) {
			jobName := strings.TrimPrefix(label, jobPrefix)
			jobMapVolumeJob[jobName] = &longhorn.VolumeRecurringJob{
				Name:    jobName,
				IsGroup: false,
			}
		}
	}
	return jobMapVolumeJob
}

// AddRecurringJobLabelToVolume adds a recurring job label to the given volume.
func (s *DataStore) AddRecurringJobLabelToVolume(volume *longhorn.Volume, labelKey string) (*longhorn.Volume, error) {
	var err error
	if _, exist := volume.Labels[labelKey]; !exist {
		volume.Labels[labelKey] = types.LonghornLabelValueEnabled
		volume, err = s.UpdateVolume(volume)
		if err != nil {
			return nil, err
		}
		logrus.Infof("Added volume %v recurring job label %v", volume.Name, labelKey)
	}
	return volume, nil
}

// RemoveRecurringJobLabelFromVolume removes a recurring job label from the given volume.
func (s *DataStore) RemoveRecurringJobLabelFromVolume(volume *longhorn.Volume, labelKey string) (*longhorn.Volume, error) {
	var err error
	if _, exist := volume.Labels[labelKey]; exist {
		delete(volume.Labels, labelKey)
		volume, err = s.UpdateVolume(volume)
		if err != nil {
			return nil, err
		}
		logrus.Infof("Removed volume %v recurring job label %v", volume.Name, labelKey)
	}
	return volume, nil
}

// ListDRVolumesRO returns a single object contains all DR Volumes
func (s *DataStore) ListDRVolumesRO() (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	list, err := s.ListVolumesRO()
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		if itemRO.Spec.Standby {
			itemMap[itemRO.Name] = itemRO
		}
	}
	return itemMap, nil
}

// ListDRVolumesWithBackupVolumeNameRO returns a single object contains the DR volumes
// matches to the backup volume name
func (s *DataStore) ListDRVolumesWithBackupVolumeNameRO(backupVolumeName string) (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	list, err := s.ListVolumesROWithBackupVolumeName(backupVolumeName)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		if itemRO.Spec.Standby {
			itemMap[itemRO.Name] = itemRO
		}
	}
	return itemMap, nil
}

// ListVolumesByLabelSelector returns an object contains all Volume
func (s *DataStore) ListVolumesByLabelSelector(selector labels.Selector) (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	list, err := s.ListVolumesBySelectorRO(selector)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListVolumesFollowsGlobalSettingsRO returns an object contains all Volumes
// that don't apply specific spec and follow the global settings
func (s *DataStore) ListVolumesFollowsGlobalSettingsRO(followedSettingNames map[string]bool) (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	followedGlobalSettingsLabels := map[string]string{}
	for sName := range followedSettingNames {
		if types.SettingsRelatedToVolume[sName] != types.LonghornLabelValueIgnored {
			continue
		}
		followedGlobalSettingsLabels[types.GetVolumeSettingLabelKey(sName)] = types.LonghornLabelValueIgnored
	}
	selectors, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: followedGlobalSettingsLabels,
	})
	if err != nil {
		return nil, err
	}

	list, err := s.ListVolumesBySelectorRO(selectors)
	if err != nil {
		return nil, err
	}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}

	return itemMap, nil
}

// ListVolumesByBackupVolumeRO returns an object contains all Volumes with the specified backup-volume label
func (s *DataStore) ListVolumesByBackupVolumeRO(backupVolumeName string) (map[string]*longhorn.Volume, error) {
	itemMap := make(map[string]*longhorn.Volume)

	labels := map[string]string{
		types.LonghornLabelBackupVolume: backupVolumeName,
	}

	selectors, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: labels,
	})
	if err != nil {
		return nil, err
	}

	list, err := s.ListVolumesBySelectorRO(selectors)
	if err != nil {
		return nil, err
	}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}

	return itemMap, nil
}

// GetLabelsForVolumesFollowsGlobalSettings returns a label map in which
// contains all global settings the volume follows
func GetLabelsForVolumesFollowsGlobalSettings(volume *longhorn.Volume) map[string]string {
	followedGlobalSettingsLabels := map[string]string{}
	// The items here should match types.SettingsRelatedToVolume
	if volume.Spec.ReplicaAutoBalance == longhorn.ReplicaAutoBalanceIgnored {
		followedGlobalSettingsLabels[types.GetVolumeSettingLabelKey(string(types.SettingNameReplicaAutoBalance))] = types.LonghornLabelValueIgnored
	}
	if volume.Spec.SnapshotDataIntegrity == longhorn.SnapshotDataIntegrityIgnored {
		followedGlobalSettingsLabels[types.GetVolumeSettingLabelKey(string(types.SettingNameSnapshotDataIntegrity))] = types.LonghornLabelValueIgnored
	}
	if volume.Spec.UnmapMarkSnapChainRemoved == longhorn.UnmapMarkSnapChainRemovedIgnored {
		followedGlobalSettingsLabels[types.GetVolumeSettingLabelKey(string(types.SettingNameRemoveSnapshotsDuringFilesystemTrim))] = types.LonghornLabelValueIgnored
	}

	return followedGlobalSettingsLabels
}

func checkEngine(engine *longhorn.Engine) error {
	if engine.Name == "" || engine.Spec.VolumeName == "" {
		return fmt.Errorf("BUG: missing required field %+v", engine)
	}
	return nil
}

// GetCurrentEngineAndExtras pick the current Engine and extra Engines from the Engine list of a volume with the given namespace
func GetCurrentEngineAndExtras(v *longhorn.Volume, es map[string]*longhorn.Engine) (currentEngine *longhorn.Engine, extras []*longhorn.Engine, err error) {
	for _, e := range es {
		if e.Spec.Active {
			if currentEngine != nil {
				return nil, nil, fmt.Errorf("BUG: found the second active engine %v besides %v", e.Name, currentEngine.Name)
			}
			currentEngine = e
		} else {
			extras = append(extras, e)
		}
	}
	if currentEngine == nil {
		logrus.Warnf("failed to directly pick up the current one from multiple engines for volume %v, fall back to detect the new current engine, "+
			"current node %v, desire node %v", v.Name, v.Status.CurrentNodeID, v.Spec.NodeID)
		newCurrentEngine, extras, err := GetNewCurrentEngineAndExtras(v, es)
		if err != nil {
			return nil, nil, err
		}
		newCurrentEngine.Spec.Active = true
		return newCurrentEngine, extras, nil
	}
	return
}

// GetNewCurrentEngineAndExtras detects the new current Engine and extra Engines from the Engine list of a volume with the given namespace during engine switching.
func GetNewCurrentEngineAndExtras(v *longhorn.Volume, es map[string]*longhorn.Engine) (currentEngine *longhorn.Engine, extras []*longhorn.Engine, err error) {
	oldEngineName := ""
	for name := range es {
		e := es[name]
		if e.Spec.Active {
			oldEngineName = e.Name
		}
		if e.DeletionTimestamp != nil {
			continue // We cannot use a deleted engine.
		}
		if (v.Spec.NodeID != "" && v.Spec.NodeID == e.Spec.NodeID) ||
			(v.Status.CurrentNodeID != "" && v.Status.CurrentNodeID == e.Spec.NodeID) {
			if currentEngine != nil {
				return nil, nil, fmt.Errorf("BUG: found the second new active engine %v besides %v", e.Name, currentEngine.Name)
			}
			currentEngine = e
		} else {
			extras = append(extras, e)
		}
	}

	// Corner case:
	// During a migration confirmation, we make the following requests to the API server:
	// 1. Delete the active engine (guaranteed to succeed or we don't do anything else).
	// 2. Switch the new current replicas to active (might fail, preventing everything below).
	// 3. Set the new current engine to active (might fail, preventing everything below).
	// 4. Set the volume.Status.CurrentNodeID (might fail).
	// So we might delete the active engine, fail to set the new current engine to active, and fail to set the
	// volume.Spec.CurrentNodeID. Then, the volume attachment controller might try to do a full detachment, setting
	// volume.Spec.NodeID == "". At this point, there is no engine that can ever be considered active again by the rules
	// above, but we want to use the new current engine. It's engine.Spec.NodeID ==
	// volume.Status.CurrentMigrationNodeID.
	if currentEngine == nil {
		extrasCopy := extras
		extras = []*longhorn.Engine{}
		for _, e := range extrasCopy {
			if e.Spec.NodeID != "" && e.Spec.NodeID == v.Status.CurrentMigrationNodeID {
				currentEngine = e
			} else {
				extras = append(extras, e)
			}
		}
	}

	if currentEngine == nil {
		return nil, nil, fmt.Errorf("cannot find the current engine for the switching after iterating and cleaning up all engines for volume %v, all engines may be detached or in a transient state", v.Name)
	}

	if currentEngine.Name != oldEngineName {
		logrus.Infof("Found the new current engine %v for volume %v, the old one is %v", currentEngine.Name, v.Name, oldEngineName)
	} else {
		logrus.Infof("The current engine for volume %v is still %v", v.Name, currentEngine.Name)
	}

	return
}

// PickVolumeCurrentEngine pick the current Engine from the Engine list of a volume with the given namespace
func (s *DataStore) PickVolumeCurrentEngine(v *longhorn.Volume, es map[string]*longhorn.Engine) (*longhorn.Engine, error) {
	if len(es) == 0 {
		return nil, nil
	}

	e, _, err := GetCurrentEngineAndExtras(v, es)
	return e, err
}

// GetVolumeCurrentEngine returns the Engine for a volume with the given namespace
func (s *DataStore) GetVolumeCurrentEngine(volumeName string) (*longhorn.Engine, error) {
	es, err := s.ListVolumeEnginesRO(volumeName)
	if err != nil {
		return nil, err
	}
	v, err := s.GetVolumeRO(volumeName)
	if err != nil {
		return nil, err
	}
	e, err := s.PickVolumeCurrentEngine(v, es)
	if err != nil {
		return nil, err
	}
	return e.DeepCopy(), nil
}

// CreateEngine creates a Longhorn Engine resource and verifies creation
func (s *DataStore) CreateEngine(e *longhorn.Engine) (*longhorn.Engine, error) {
	if err := checkEngine(e); err != nil {
		return nil, err
	}
	if err := labelNode(e.Spec.NodeID, e); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).Create(context.TODO(), e, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(e.Name, "engine", func(name string) (k8sruntime.Object, error) {
		return s.GetEngineRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Engine)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for engine")
	}

	return ret.DeepCopy(), nil
}

// UpdateEngine updates Longhorn Engine and verifies update
func (s *DataStore) UpdateEngine(e *longhorn.Engine) (*longhorn.Engine, error) {
	if err := checkEngine(e); err != nil {
		return nil, err
	}
	if err := labelNode(e.Spec.NodeID, e); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).Update(context.TODO(), e, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(e.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetEngineRO(name)
	})
	return obj, nil
}

// UpdateEngineStatus updates Longhorn Engine status and verifies update
func (s *DataStore) UpdateEngineStatus(e *longhorn.Engine) (*longhorn.Engine, error) {
	obj, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).UpdateStatus(context.TODO(), e, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(e.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetEngineRO(name)
	})
	return obj, nil
}

// DeleteEngine won't result in immediately deletion since finalizer was set by
// default
func (s *DataStore) DeleteEngine(name string) error {
	return s.lhClient.LonghornV1beta2().Engines(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForEngine will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForEngine(obj *longhorn.Engine) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for engine %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetEngineRO(name string) (*longhorn.Engine, error) {
	return s.engineLister.Engines(s.namespace).Get(name)
}

// GetEngine returns the Engine for the given name and namespace
func (s *DataStore) GetEngine(name string) (*longhorn.Engine, error) {
	resultRO, err := s.GetEngineRO(name)
	if err != nil {
		return nil, err
	}

	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) listEngines(selector labels.Selector) (map[string]*longhorn.Engine, error) {
	list, err := s.engineLister.Engines(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}
	engines := map[string]*longhorn.Engine{}
	for _, e := range list {
		// Cannot use cached object from lister
		engines[e.Name] = e.DeepCopy()
	}
	return engines, nil
}

// ListEngines returns an object contains all Engine for the given namespace
func (s *DataStore) ListEngines() (map[string]*longhorn.Engine, error) {
	return s.listEngines(labels.Everything())
}

// ListEnginesRO returns a list of all Engine for the given namespace
func (s *DataStore) ListEnginesRO() ([]*longhorn.Engine, error) {
	return s.engineLister.Engines(s.namespace).List(labels.Everything())
}

// ListVolumeEngines returns an object contains all Engines with the given
// LonghornLabelVolume name and namespace
func (s *DataStore) ListVolumeEngines(volumeName string) (map[string]*longhorn.Engine, error) {
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}
	return s.listEngines(selector)
}

func (s *DataStore) ListVolumeEnginesRO(volumeName string) (map[string]*longhorn.Engine, error) {
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}

	engineList, err := s.engineLister.Engines(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	engineMap := make(map[string]*longhorn.Engine, len(engineList))
	for _, e := range engineList {
		engineMap[e.Name] = e
	}

	return engineMap, nil
}

func checkReplica(r *longhorn.Replica) error {
	if r.Name == "" || r.Spec.VolumeName == "" {
		return fmt.Errorf("BUG: missing required field %+v", r)
	}
	if (r.Status.CurrentState == longhorn.InstanceStateRunning) != (r.Status.IP != "") {
		return fmt.Errorf("BUG: instance state and IP wasn't in sync %+v", r)
	}
	if (r.Status.CurrentState == longhorn.InstanceStateRunning) != (r.Status.StorageIP != "") {
		return fmt.Errorf("BUG: instance state and storage IP wasn't in sync %+v", r)
	}
	return nil
}

// CreateReplica creates a Longhorn Replica resource and verifies creation
func (s *DataStore) CreateReplica(r *longhorn.Replica) (*longhorn.Replica, error) {
	if err := checkReplica(r); err != nil {
		return nil, err
	}
	if err := labelNode(r.Spec.NodeID, r); err != nil {
		return nil, err
	}
	if err := labelDiskUUID(r.Spec.DiskID, r); err != nil {
		return nil, err
	}
	if err := labelBackingImage(r.Spec.BackingImage, r); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).Create(context.TODO(), r, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "replica", func(name string) (k8sruntime.Object, error) {
		return s.GetReplicaRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Replica)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for replica")
	}

	return ret.DeepCopy(), nil
}

// UpdateReplica updates Replica and verifies update
func (s *DataStore) UpdateReplica(r *longhorn.Replica) (*longhorn.Replica, error) {
	if err := checkReplica(r); err != nil {
		return nil, err
	}
	if err := labelNode(r.Spec.NodeID, r); err != nil {
		return nil, err
	}
	if err := labelDiskUUID(r.Spec.DiskID, r); err != nil {
		return nil, err
	}
	if err := labelBackingImage(r.Spec.BackingImage, r); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).Update(context.TODO(), r, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(r.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetReplicaRO(name)
	})
	return obj, nil
}

// UpdateReplicaStatus updates Replica status and verifies update
func (s *DataStore) UpdateReplicaStatus(r *longhorn.Replica) (*longhorn.Replica, error) {
	if err := checkReplica(r); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).UpdateStatus(context.TODO(), r, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(r.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetReplicaRO(name)
	})
	return obj, nil
}

// DeleteReplica won't result in immediately deletion since finalizer was set
// by default
func (s *DataStore) DeleteReplica(name string) error {
	return s.lhClient.LonghornV1beta2().Replicas(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForReplica will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForReplica(obj *longhorn.Replica) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for replica %v", obj.Name)
	}
	return nil
}

// GetReplica gets Replica for the given name and namespace and returns
// a new Replica object
func (s *DataStore) GetReplica(name string) (*longhorn.Replica, error) {
	resultRO, err := s.GetReplicaRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) GetReplicaRO(name string) (*longhorn.Replica, error) {
	return s.replicaLister.Replicas(s.namespace).Get(name)
}

func (s *DataStore) listReplicas(selector labels.Selector) (map[string]*longhorn.Replica, error) {
	list, err := s.replicaLister.Replicas(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Replica{}
	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListReplicas returns an object contains all Replicas for the given namespace
func (s *DataStore) ListReplicas() (map[string]*longhorn.Replica, error) {
	return s.listReplicas(labels.Everything())
}

// ListReplicasRO returns a list of all replicas for the given namespace
func (s *DataStore) ListReplicasRO() ([]*longhorn.Replica, error) {
	return s.replicaLister.Replicas(s.namespace).List(labels.Everything())
}

// ListVolumeReplicas returns an object contains all Replica with the given
// LonghornLabelVolume name and namespace
func (s *DataStore) ListVolumeReplicas(volumeName string) (map[string]*longhorn.Replica, error) {
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}
	return s.listReplicas(selector)
}

func (s *DataStore) ListVolumeReplicasRO(volumeName string) (map[string]*longhorn.Replica, error) {
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}

	rList, err := s.replicaLister.Replicas(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	rMap := make(map[string]*longhorn.Replica, len(rList))
	for _, rRO := range rList {
		rMap[rRO.Name] = rRO
	}

	return rMap, nil
}

// ListVolumeReplicasROMapByNode returns a map of read-only replicas grouped by
// the node ID for the given volume.
// The function organizes the replicas into a map where the keys is the node ID,
// and the values are maps of replica names to the replica objects.
// If successful, the function returns the map of replicas by node. Otherwise,
// an error is returned.
func (s *DataStore) ListVolumeReplicasROMapByNode(volumeName string) (map[string]map[string]*longhorn.Replica, error) {
	// Get volume selector
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}

	// List replicas based on the volume selector
	replicaList, err := s.replicaLister.Replicas(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	// Organize the replicas by node
	replicaMapByNode := make(map[string]map[string]*longhorn.Replica)
	for _, replica := range replicaList {
		nodeID := replica.Spec.NodeID

		// Create the map if it doesn't exist for the current replica's node
		if _, exists := replicaMapByNode[nodeID]; !exists {
			replicaMapByNode[nodeID] = make(map[string]*longhorn.Replica)
		}

		// Add the replica to the map
		replicaMapByNode[nodeID][replica.Name] = replica
	}

	return replicaMapByNode, nil
}

// ReplicaAddressToReplicaName will directly return the address if the format
// is invalid or the replica is not found.
func ReplicaAddressToReplicaName(address string, rs []*longhorn.Replica) string {
	addressComponents := strings.Split(strings.TrimPrefix(address, "tcp://"), ":")
	// The address format should be `<IP>:<Port>` after removing the prefix "tcp://".
	if len(addressComponents) != 2 {
		return address
	}
	for _, r := range rs {
		if addressComponents[0] == r.Status.StorageIP && addressComponents[1] == strconv.Itoa(r.Status.Port) {
			return r.Name
		}
	}
	// Cannot find matching replica by the address, replica may be removed already. Use address instead.
	return address
}

// IsAvailableHealthyReplica returns if the specified replica is a healthy one
func IsAvailableHealthyReplica(r *longhorn.Replica) bool {
	if r == nil {
		return false
	}
	if r.DeletionTimestamp != nil {
		return false
	}
	if r.Spec.FailedAt != "" || r.Spec.HealthyAt == "" {
		return false
	}
	return true
}

func (s *DataStore) ListVolumePDBProtectedHealthyReplicasRO(volumeName string) (map[string]*longhorn.Replica, error) {
	pdbProtectedHealthyReplicas := map[string]*longhorn.Replica{}
	replicas, err := s.ListVolumeReplicasRO(volumeName)
	if err != nil {
		return nil, err
	}

	for _, replica := range replicas {
		if replica.Spec.HealthyAt == "" || replica.Spec.FailedAt != "" {
			continue
		}

		unschedulable, err := s.IsKubeNodeUnschedulable(replica.Spec.NodeID)
		if err != nil {
			return map[string]*longhorn.Replica{}, err
		}
		if unschedulable {
			continue
		}

		instanceManager, err := s.getRunningReplicaInstanceManagerRO(replica)
		if err != nil {
			return map[string]*longhorn.Replica{}, err
		}
		if instanceManager == nil {
			continue
		}

		pdb, err := s.GetPDBRO(types.GetPDBName(instanceManager))
		if err != nil && !ErrorIsNotFound(err) {
			return map[string]*longhorn.Replica{}, err
		}
		if pdb != nil {
			pdbProtectedHealthyReplicas[replica.Name] = replica
		}
	}

	return pdbProtectedHealthyReplicas, nil
}

func (s *DataStore) getRunningReplicaInstanceManagerRO(r *longhorn.Replica) (im *longhorn.InstanceManager, err error) {
	if r.Status.InstanceManagerName == "" {
		im, err = s.GetInstanceManagerByInstanceRO(r)
		if err != nil && !types.ErrorIsNotFound(err) {
			return nil, err
		}
	} else {
		im, err = s.GetInstanceManagerRO(r.Status.InstanceManagerName)
		if err != nil && !ErrorIsNotFound(err) {
			return nil, err
		}
	}
	if im == nil || im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return nil, nil
	}
	return im, nil
}

// IsReplicaRebuildingFailed returns true if the rebuilding replica failed not caused by network issues.
func IsReplicaRebuildingFailed(reusableFailedReplica *longhorn.Replica) bool {
	replicaRebuildFailedCondition := types.GetCondition(reusableFailedReplica.Status.Conditions, longhorn.ReplicaConditionTypeRebuildFailed)

	if replicaRebuildFailedCondition.Status != longhorn.ConditionStatusTrue {
		return true
	}

	switch replicaRebuildFailedCondition.Reason {
	case longhorn.ReplicaConditionReasonRebuildFailedDisconnection, longhorn.NodeConditionReasonManagerPodDown, longhorn.NodeConditionReasonKubernetesNodeGone, longhorn.NodeConditionReasonKubernetesNodeNotReady:
		return false
	default:
		return true
	}
}

// GetOwnerReferencesForEngineImage returns OwnerReference for the given
// Longhorn EngineImage name and UID
func GetOwnerReferencesForEngineImage(ei *longhorn.EngineImage) []metav1.OwnerReference {
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion:         longhorn.SchemeGroupVersion.String(),
			Kind:               types.LonghornKindEngineImage,
			Name:               ei.Name,
			UID:                ei.UID,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// CreateEngineImage creates a Longhorn EngineImage resource and verifies
// creation
func (s *DataStore) CreateEngineImage(img *longhorn.EngineImage) (*longhorn.EngineImage, error) {
	ret, err := s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Create(context.TODO(), img, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "engine image", func(name string) (k8sruntime.Object, error) {
		return s.GetEngineImageRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.EngineImage)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for engine image")
	}

	return ret.DeepCopy(), nil
}

// UpdateEngineImage updates Longhorn EngineImage and verifies update
func (s *DataStore) UpdateEngineImage(img *longhorn.EngineImage) (*longhorn.EngineImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Update(context.TODO(), img, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(img.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetEngineImageRO(name)
	})
	return obj, nil
}

// UpdateEngineImageStatus updates Longhorn EngineImage resource status and
// verifies update
func (s *DataStore) UpdateEngineImageStatus(img *longhorn.EngineImage) (*longhorn.EngineImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().EngineImages(s.namespace).UpdateStatus(context.TODO(), img, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(img.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetEngineImageRO(name)
	})
	return obj, nil
}

// DeleteEngineImage won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteEngineImage(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// RemoveFinalizerForEngineImage will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForEngineImage(obj *longhorn.EngineImage) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for engine image %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetEngineImageRO(name string) (*longhorn.EngineImage, error) {
	return s.engineImageLister.EngineImages(s.namespace).Get(name)
}

// GetEngineImage returns a new EngineImage object for the given name and
// namespace
func (s *DataStore) GetEngineImage(name string) (*longhorn.EngineImage, error) {
	resultRO, err := s.GetEngineImageRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) GetEngineImageByImage(image string) (*longhorn.EngineImage, error) {
	engineImages, err := s.ListEngineImages()
	if err != nil {
		return nil, err
	}

	for _, ei := range engineImages {
		if ei.Spec.Image == image {
			return ei, nil
		}
	}

	return nil, errors.Errorf("cannot find engine image by %v", image)
}

// ListEngineImages returns object includes all EngineImage in namespace
func (s *DataStore) ListEngineImages() (map[string]*longhorn.EngineImage, error) {
	itemMap := map[string]*longhorn.EngineImage{}

	list, err := s.engineImageLister.EngineImages(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) CheckDataEngineImageCompatiblityByImage(image string, dataEngine longhorn.DataEngineType) error {
	if types.IsDataEngineV2(dataEngine) {
		return nil
	}

	engineImage, err := s.GetEngineImageByImage(image)
	if err != nil {
		return err
	}

	if engineImage.Status.Incompatible {
		return errors.Errorf("engine image %v is incompatible", engineImage.Name)
	}

	return nil
}

// CheckEngineImageReadiness return true if the engine IMAGE is deployed on all nodes in the NODES list
func (s *DataStore) CheckEngineImageReadiness(image string, nodes ...string) (isReady bool, err error) {
	if len(nodes) == 0 {
		return false, nil
	}
	if len(nodes) == 1 && nodes[0] == "" {
		return false, nil
	}
	ei, err := s.GetEngineImageRO(types.GetEngineImageChecksumName(image))
	if err != nil {
		return false, errors.Wrapf(err, "failed to get engine image %v", image)
	}
	if ei.Status.State != longhorn.EngineImageStateDeployed &&
		ei.Status.State != longhorn.EngineImageStateDeploying {
		logrus.Warnf("CheckEngineImageReadiness: engine image %v is in state %v", image, ei.Status.State)
		return false, nil
	}
	nodesHaveEngineImage, err := s.ListNodesContainingEngineImageRO(ei)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list nodes containing engine image %v", image)
	}
	undeployedNodes := []string{}
	for _, node := range nodes {
		if node == "" {
			continue
		}
		if _, ok := nodesHaveEngineImage[node]; !ok {
			undeployedNodes = append(undeployedNodes, node)
		}
	}
	if len(undeployedNodes) > 0 {
		logrus.Infof("CheckEngineImageReadiness: nodes %v don't have the engine image %v", undeployedNodes, image)
		return false, nil
	}
	return true, nil
}

func (s *DataStore) CheckDataEngineImageReadiness(image string, dataEngine longhorn.DataEngineType, nodes ...string) (isReady bool, err error) {
	if types.IsDataEngineV2(dataEngine) {
		if len(nodes) == 0 {
			return false, nil
		}
		if len(nodes) == 1 && nodes[0] == "" {
			return false, nil
		}
		return true, nil
	}
	// For data engine v1, we need to check the engine image readiness
	return s.CheckEngineImageReadiness(image, nodes...)
}

// CheckDataEngineImageReadyOnAtLeastOneVolumeReplica checks if the IMAGE is deployed on the NODEID and on at least one of the the volume's replicas
func (s *DataStore) CheckDataEngineImageReadyOnAtLeastOneVolumeReplica(image, volumeName, nodeID string, dataLocality longhorn.DataLocality, dataEngine longhorn.DataEngineType) (bool, error) {
	isReady, err := s.CheckDataEngineImageReadiness(image, dataEngine, nodeID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check data engine image readiness of node %v", nodeID)
	}

	if !isReady || dataLocality == longhorn.DataLocalityStrictLocal {
		return isReady, nil
	}

	replicas, err := s.ListVolumeReplicas(volumeName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get replicas for volume %v", volumeName)
	}

	hasScheduledReplica := false
	for _, r := range replicas {
		isReady, err := s.CheckDataEngineImageReadiness(image, r.Spec.DataEngine, r.Spec.NodeID)
		if err != nil || isReady {
			return isReady, err
		}
		if r.Spec.NodeID != "" {
			hasScheduledReplica = true
		}
	}
	if !hasScheduledReplica {
		return false, errors.Errorf("volume %v has no scheduled replicas", volumeName)
	}
	return false, nil
}

// CheckImageReadyOnAllVolumeReplicas checks if the IMAGE is deployed on the NODEID as well as all the volume's replicas
func (s *DataStore) CheckImageReadyOnAllVolumeReplicas(image, volumeName, nodeID string, dataEngine longhorn.DataEngineType) (bool, error) {
	replicas, err := s.ListVolumeReplicas(volumeName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get replicas for volume %v", volumeName)
	}
	nodes := []string{}
	if nodeID != "" {
		nodes = append(nodes, nodeID)
	}
	for _, r := range replicas {
		if r.Spec.NodeID != "" {
			nodes = append(nodes, r.Spec.NodeID)
		}
	}
	return s.CheckDataEngineImageReadiness(image, dataEngine, nodes...)
}

// CreateBackingImage creates a Longhorn BackingImage resource and verifies
// creation
func (s *DataStore) CreateBackingImage(backingImage *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	ret, err := s.lhClient.LonghornV1beta2().BackingImages(s.namespace).Create(context.TODO(), backingImage, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backing image", func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackingImage)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for backing image")
	}

	return ret.DeepCopy(), nil
}

// UpdateBackingImage updates Longhorn BackingImage and verifies update
func (s *DataStore) UpdateBackingImage(backingImage *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackingImages(s.namespace).Update(context.TODO(), backingImage, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImage.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageRO(name)
	})
	return obj, nil
}

// UpdateBackingImageStatus updates Longhorn BackingImage resource status and
// verifies update
func (s *DataStore) UpdateBackingImageStatus(backingImage *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackingImages(s.namespace).UpdateStatus(context.TODO(), backingImage, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImage.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageRO(name)
	})
	return obj, nil
}

// DeleteBackingImage won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteBackingImage(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().BackingImages(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// RemoveFinalizerForBackingImage will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackingImage(obj *longhorn.BackingImage) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackingImages(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backing image %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetBackingImageRO(name string) (*longhorn.BackingImage, error) {
	return s.backingImageLister.BackingImages(s.namespace).Get(name)
}

// GetBackingImage returns a new BackingImage object for the given name and
// namespace
func (s *DataStore) GetBackingImage(name string) (*longhorn.BackingImage, error) {
	resultRO, err := s.GetBackingImageRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListBackingImages returns object includes all BackingImage in namespace
func (s *DataStore) ListBackingImages() (map[string]*longhorn.BackingImage, error) {
	itemMap := map[string]*longhorn.BackingImage{}

	list, err := s.backingImageLister.BackingImages(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListBackingImagesRO returns object includes all BackingImage in namespace
func (s *DataStore) ListBackingImagesRO() ([]*longhorn.BackingImage, error) {
	return s.backingImageLister.BackingImages(s.namespace).List(labels.Everything())
}

// GetOwnerReferencesForBackingImage returns OwnerReference for the given
// backing image name and UID
func GetOwnerReferencesForBackingImage(backingImage *longhorn.BackingImage) []metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion:         longhorn.SchemeGroupVersion.String(),
			Kind:               types.LonghornKindBackingImage,
			Name:               backingImage.Name,
			UID:                backingImage.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// CreateBackingImageManager creates a Longhorn BackingImageManager resource and verifies
// creation
func (s *DataStore) CreateBackingImageManager(backingImageManager *longhorn.BackingImageManager) (*longhorn.BackingImageManager, error) {
	if err := initBackingImageManager(backingImageManager); err != nil {
		return nil, err
	}
	if err := labelLonghornNode(backingImageManager.Spec.NodeID, backingImageManager); err != nil {
		return nil, err
	}
	if err := labelLonghornDiskUUID(backingImageManager.Spec.DiskUUID, backingImageManager); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Create(context.TODO(), backingImageManager, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backing image manager", func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageManagerRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackingImageManager)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for backing image manager")
	}

	return ret.DeepCopy(), nil
}

func initBackingImageManager(backingImageManager *longhorn.BackingImageManager) error {
	if backingImageManager.Spec.BackingImages == nil {
		backingImageManager.Spec.BackingImages = make(map[string]string, 0)
	}
	return nil
}

// UpdateBackingImageManager updates Longhorn BackingImageManager and verifies update
func (s *DataStore) UpdateBackingImageManager(backingImageManager *longhorn.BackingImageManager) (*longhorn.BackingImageManager, error) {
	if err := labelLonghornNode(backingImageManager.Spec.NodeID, backingImageManager); err != nil {
		return nil, err
	}
	if err := labelLonghornDiskUUID(backingImageManager.Spec.DiskUUID, backingImageManager); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Update(context.TODO(), backingImageManager, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageManager.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageManagerRO(name)
	})
	return obj, nil
}

// UpdateBackingImageManagerStatus updates Longhorn BackingImageManager resource status and
// verifies update
func (s *DataStore) UpdateBackingImageManagerStatus(backingImageManager *longhorn.BackingImageManager) (*longhorn.BackingImageManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).UpdateStatus(context.TODO(), backingImageManager, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageManager.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackingImageManagerRO(name)
	})
	return obj, nil
}

// DeleteBackingImageManager won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteBackingImageManager(name string) error {
	return s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForBackingImageManager will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackingImageManager(obj *longhorn.BackingImageManager) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backing image manager %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetBackingImageManagerRO(name string) (*longhorn.BackingImageManager, error) {
	return s.backingImageManagerLister.BackingImageManagers(s.namespace).Get(name)
}

// GetBackingImageManager returns a new BackingImageManager object for the given name and
// namespace
func (s *DataStore) GetBackingImageManager(name string) (*longhorn.BackingImageManager, error) {
	resultRO, err := s.GetBackingImageManagerRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) ListBackingImageManagersRO() ([]*longhorn.BackingImageManager, error) {
	return s.listBackingImageManagersRO(labels.Everything())
}

func (s *DataStore) listBackingImageManagers(selector labels.Selector) (map[string]*longhorn.BackingImageManager, error) {
	itemMap := map[string]*longhorn.BackingImageManager{}

	list, err := s.listBackingImageManagersRO(selector)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) listBackingImageManagersRO(selector labels.Selector) ([]*longhorn.BackingImageManager, error) {
	return s.backingImageManagerLister.BackingImageManagers(s.namespace).List(selector)
}

// ListBackingImageManagers returns object includes all BackingImageManager in namespace
func (s *DataStore) ListBackingImageManagers() (map[string]*longhorn.BackingImageManager, error) {
	return s.listBackingImageManagers(labels.Everything())
}

// ListBackingImageManagersByNode gets a list of BackingImageManager
// on a specific node with the given namespace.
func (s *DataStore) ListBackingImageManagersByNode(nodeName string) (map[string]*longhorn.BackingImageManager, error) {
	nodeSelector, err := getLonghornNodeSelector(nodeName)
	if err != nil {
		return nil, err
	}
	return s.listBackingImageManagers(nodeSelector)
}

func (s *DataStore) ListBackingImageManagersByNodeRO(nodeName string) ([]*longhorn.BackingImageManager, error) {
	nodeSelector, err := getLonghornNodeSelector(nodeName)
	if err != nil {
		return nil, err
	}
	return s.listBackingImageManagersRO(nodeSelector)
}

// ListBackingImageManagersByDiskUUID gets a list of BackingImageManager
// in a specific disk with the given namespace.
func (s *DataStore) ListBackingImageManagersByDiskUUID(diskUUID string) (map[string]*longhorn.BackingImageManager, error) {
	diskSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.GetLonghornLabelKey(types.LonghornLabelDiskUUID): diskUUID,
		},
	})
	if err != nil {
		return nil, err
	}
	return s.listBackingImageManagers(diskSelector)
}

// GetOwnerReferencesForBackingImageManager returns OwnerReference for the given
// // backing image manager name and UID
func GetOwnerReferencesForBackingImageManager(backingImageManager *longhorn.BackingImageManager) []metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindBackingImageManager,
			Name:       backingImageManager.Name,
			UID:        backingImageManager.UID,
			// This field is needed so that `kubectl drain` can work without --force flag
			// See https://github.com/longhorn/longhorn/issues/1286#issuecomment-623283028 for more details
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// CreateBackingImageDataSource creates a Longhorn BackingImageDataSource resource and verifies
// creation
func (s *DataStore) CreateBackingImageDataSource(backingImageDataSource *longhorn.BackingImageDataSource) (*longhorn.BackingImageDataSource, error) {
	if err := initBackingImageDataSource(backingImageDataSource); err != nil {
		return nil, err
	}
	ret, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Create(context.TODO(), backingImageDataSource, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backing image data source", func(name string) (k8sruntime.Object, error) {
		return s.getBackingImageDataSourceRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackingImageDataSource)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for backing image data source")
	}

	return ret.DeepCopy(), nil
}

func initBackingImageDataSource(backingImageDataSource *longhorn.BackingImageDataSource) error {
	if backingImageDataSource.Spec.Parameters == nil {
		backingImageDataSource.Spec.Parameters = make(map[string]string, 0)
	}
	return nil
}

// UpdateBackingImageDataSource updates Longhorn BackingImageDataSource and verifies update
func (s *DataStore) UpdateBackingImageDataSource(backingImageDataSource *longhorn.BackingImageDataSource) (*longhorn.BackingImageDataSource, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Update(context.TODO(), backingImageDataSource, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageDataSource.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getBackingImageDataSourceRO(name)
	})
	return obj, nil
}

// UpdateBackingImageDataSourceStatus updates Longhorn BackingImageDataSource resource status and
// verifies update
func (s *DataStore) UpdateBackingImageDataSourceStatus(backingImageDataSource *longhorn.BackingImageDataSource) (*longhorn.BackingImageDataSource, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).UpdateStatus(context.TODO(), backingImageDataSource, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageDataSource.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getBackingImageDataSourceRO(name)
	})
	return obj, nil
}

// DeleteBackingImageDataSource won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteBackingImageDataSource(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// RemoveFinalizerForBackingImageDataSource will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackingImageDataSource(obj *longhorn.BackingImageDataSource) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backing image data source %v", obj.Name)
	}
	return nil
}

func (s *DataStore) getBackingImageDataSourceRO(name string) (*longhorn.BackingImageDataSource, error) {
	return s.backingImageDataSourceLister.BackingImageDataSources(s.namespace).Get(name)
}

// GetBackingImageDataSource returns a new BackingImageDataSource object for the given name and
// namespace
func (s *DataStore) GetBackingImageDataSource(name string) (*longhorn.BackingImageDataSource, error) {
	resultRO, err := s.getBackingImageDataSourceRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListBackingImageDataSources returns object includes all BackingImageDataSource in namespace
func (s *DataStore) ListBackingImageDataSources() (map[string]*longhorn.BackingImageDataSource, error) {
	return s.listBackingImageDataSources(labels.Everything())
}

// ListBackingImageDataSourcesExportingFromVolume returns object includes all BackingImageDataSource in namespace
func (s *DataStore) ListBackingImageDataSourcesExportingFromVolume(volumeName string) (map[string]*longhorn.BackingImageDataSource, error) {
	volumeSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.GetLonghornLabelKey(types.LonghornLabelExportFromVolume): volumeName,
		},
	})
	if err != nil {
		return nil, err
	}
	bidsList, err := s.listBackingImageDataSources(volumeSelector)
	if err != nil {
		return nil, err
	}
	exportingBackingImageDataSources := map[string]*longhorn.BackingImageDataSource{}
	for _, bids := range bidsList {
		if bids.Spec.SourceType != longhorn.BackingImageDataSourceTypeExportFromVolume || bids.Spec.FileTransferred {
			continue
		}
		if bids.Status.CurrentState == longhorn.BackingImageStateFailed || bids.Status.CurrentState == longhorn.BackingImageStateUnknown {
			continue
		}
		exportingBackingImageDataSources[bids.Name] = bids
	}
	return exportingBackingImageDataSources, nil
}

// ListBackingImageDataSourcesByNode returns object includes all BackingImageDataSource in namespace
func (s *DataStore) ListBackingImageDataSourcesByNode(nodeName string) (map[string]*longhorn.BackingImageDataSource, error) {
	nodeSelector, err := getNodeSelector(nodeName)
	if err != nil {
		return nil, err
	}
	return s.listBackingImageDataSources(nodeSelector)
}

func (s *DataStore) listBackingImageDataSources(selector labels.Selector) (map[string]*longhorn.BackingImageDataSource, error) {
	itemMap := map[string]*longhorn.BackingImageDataSource{}

	list, err := s.backingImageDataSourceLister.BackingImageDataSources(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// GetOwnerReferencesForBackingImageDataSource returns OwnerReference for the given
// backing image data source name and UID
func GetOwnerReferencesForBackingImageDataSource(backingImageDataSource *longhorn.BackingImageDataSource) []metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion:         longhorn.SchemeGroupVersion.String(),
			Kind:               types.LonghornKindBackingImageDataSource,
			Name:               backingImageDataSource.Name,
			UID:                backingImageDataSource.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// CreateNode creates a Longhorn Node resource and verifies creation
func (s *DataStore) CreateNode(node *longhorn.Node) (*longhorn.Node, error) {
	ret, err := s.lhClient.LonghornV1beta2().Nodes(s.namespace).Create(context.TODO(), node, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "node", func(name string) (k8sruntime.Object, error) {
		return s.GetNodeRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Node)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for node")
	}

	return ret.DeepCopy(), nil
}

// CreateDefaultNode will create the default Disk at the value of the
// DefaultDataPath Setting only if Create Default Disk on Labeled Nodes has
// been disabled.
func (s *DataStore) CreateDefaultNode(name string) (*longhorn.Node, error) {
	requireLabel, err := s.GetSettingAsBool(types.SettingNameCreateDefaultDiskLabeledNodes)
	if err != nil {
		return nil, err
	}
	node := &longhorn.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: longhorn.NodeSpec{
			Name:                      name,
			AllowScheduling:           true,
			EvictionRequested:         false,
			Tags:                      []string{},
			InstanceManagerCPURequest: 0,
		},
	}

	// For newly added node, the customized default disks will be applied only if the setting is enabled.
	if !requireLabel {
		// Note: this part wasn't moved to the controller is because
		// this will be done only once.
		// If user remove all the disks on the node, the default disk
		// will not be recreated automatically
		dataPath, err := s.GetSettingValueExisted(types.SettingNameDefaultDataPath)
		if err != nil {
			return nil, err
		}
		storageReservedPercentageForDefaultDisk, err := s.GetSettingAsInt(types.SettingNameStorageReservedPercentageForDefaultDisk)
		if err != nil {
			return nil, err
		}

		if s.needDefaultDiskCreation(dataPath) {
			disks, err := types.CreateDefaultDisk(dataPath, storageReservedPercentageForDefaultDisk)
			if err != nil {
				return nil, err
			}
			node.Spec.Disks = disks
		}
	}

	return s.CreateNode(node)
}

func (s *DataStore) needDefaultDiskCreation(dataPath string) bool {
	v2DataEngineEnabled, err := s.GetSettingAsBool(types.SettingNameV2DataEngine)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to get setting %v", types.SettingNameV2DataEngine)
		return false
	}

	if v2DataEngineEnabled {
		return true
	}

	// Do not create default block-type disk if v2 data engine is disabled
	return !types.IsPotentialBlockDisk(dataPath)
}

func (s *DataStore) GetNodeRO(name string) (*longhorn.Node, error) {
	return s.nodeLister.Nodes(s.namespace).Get(name)
}

// GetNode gets Longhorn Node for the given name and namespace
// Returns a new Node object
func (s *DataStore) GetNode(name string) (*longhorn.Node, error) {
	resultRO, err := s.GetNodeRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// GetReadyDiskNode find the corresponding ready Longhorn Node for a given disk
// Returns a Node object and the disk name
func (s *DataStore) GetReadyDiskNode(diskUUID string) (*longhorn.Node, string, error) {
	node, diskName, err := s.GetReadyDiskNodeRO(diskUUID)
	if err != nil {
		return nil, "", err
	}

	return node.DeepCopy(), diskName, nil
}

func (s *DataStore) GetReadyDiskNodeRO(diskUUID string) (*longhorn.Node, string, error) {
	nodes, err := s.ListReadyNodesRO()
	if err != nil {
		return nil, "", err
	}
	for _, node := range nodes {
		for diskName, diskStatus := range node.Status.DiskStatus {
			if diskStatus.DiskUUID == diskUUID {
				condition := types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady)
				if condition.Status == longhorn.ConditionStatusTrue {
					if _, exists := node.Spec.Disks[diskName]; exists {
						return node, diskName, nil
					}
				}
			}
		}
	}

	return nil, "", fmt.Errorf("cannot find the corresponding ready node and disk with disk UUID %v", diskUUID)
}

// GetReadyDisk find disk name by the given nodeName and diskUUD
// Returns a disk name
func (s *DataStore) GetReadyDisk(nodeName string, diskUUID string) (string, error) {
	node, err := s.GetNodeRO(nodeName)
	if err != nil {
		return "", err
	}

	for diskName, diskStatus := range node.Status.DiskStatus {
		if diskStatus.DiskUUID == diskUUID {
			condition := types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeReady)
			if condition.Status == longhorn.ConditionStatusTrue {
				if _, exists := node.Spec.Disks[diskName]; exists {
					return diskName, nil
				}
			}
		}
	}

	return "", fmt.Errorf("cannot find the ready disk with UUID %v", diskUUID)
}

// UpdateNode updates Longhorn Node resource and verifies update
func (s *DataStore) UpdateNode(node *longhorn.Node) (*longhorn.Node, error) {
	obj, err := s.lhClient.LonghornV1beta2().Nodes(s.namespace).Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(node.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetNodeRO(name)
	})
	return obj, nil
}

// UpdateNodeStatus updates Longhorn Node status and verifies update
func (s *DataStore) UpdateNodeStatus(node *longhorn.Node) (*longhorn.Node, error) {
	obj, err := s.lhClient.LonghornV1beta2().Nodes(s.namespace).UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(node.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetNodeRO(name)
	})
	return obj, nil
}

// ListNodes returns an object contains all Node for the namespace
func (s *DataStore) ListNodes() (map[string]*longhorn.Node, error) {
	itemMap := make(map[string]*longhorn.Node)

	nodeList, err := s.ListNodesRO()
	if err != nil {
		return nil, err
	}

	for _, node := range nodeList {
		// Cannot use cached object from lister
		itemMap[node.Name] = node.DeepCopy()
	}
	return itemMap, nil
}

// ListNodesRO returns a list of all Nodes for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListNodesRO() ([]*longhorn.Node, error) {
	return s.nodeLister.Nodes(s.namespace).List(labels.Everything())
}

func (s *DataStore) ListNodesContainingEngineImageRO(ei *longhorn.EngineImage) (map[string]*longhorn.Node, error) {
	nodeList, err := s.ListNodesRO()
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*longhorn.Node)
	for _, node := range nodeList {
		if !ei.Status.NodeDeploymentMap[node.Name] {
			continue
		}
		nodeMap[node.Name] = node
	}

	return nodeMap, nil
}

// filterNodes returns only the nodes where the passed predicate returns true
func filterNodes(nodes map[string]*longhorn.Node, predicate func(node *longhorn.Node) bool) map[string]*longhorn.Node {
	filtered := make(map[string]*longhorn.Node)
	for nodeID, node := range nodes {
		if predicate(node) {
			filtered[nodeID] = node
		}
	}
	return filtered
}

// filterReadyNodes returns only the nodes that are ready
func filterReadyNodes(nodes map[string]*longhorn.Node) map[string]*longhorn.Node {
	return filterNodes(nodes, func(node *longhorn.Node) bool {
		nodeReadyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		return nodeReadyCondition.Status == longhorn.ConditionStatusTrue
	})
}

// filterSchedulableNodes returns only the nodes that are ready and have at least one schedulable disk
func filterSchedulableNodes(nodes map[string]*longhorn.Node) map[string]*longhorn.Node {
	return filterNodes(nodes, func(node *longhorn.Node) bool {
		nodeSchedulableCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable)
		if nodeSchedulableCondition.Status != longhorn.ConditionStatusTrue {
			return false
		}

		for _, disk := range node.Spec.Disks {
			if disk.EvictionRequested {
				continue
			}

			if disk.AllowScheduling {
				return true
			}
		}
		return false
	})
}

func (s *DataStore) ListReadyNodes() (map[string]*longhorn.Node, error) {
	nodes, err := s.ListNodes()
	if err != nil {
		return nil, err
	}
	readyNodes := filterReadyNodes(nodes)
	return readyNodes, nil
}

func (s *DataStore) ListReadyNodesRO() (map[string]*longhorn.Node, error) {
	nodeList, err := s.ListNodesRO()
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*longhorn.Node, len(nodeList))
	for _, node := range nodeList {
		nodeMap[node.Name] = node
	}
	readyNodes := filterReadyNodes(nodeMap)
	return readyNodes, nil
}

func (s *DataStore) ListReadyAndSchedulableNodesRO() (map[string]*longhorn.Node, error) {
	nodes, err := s.ListReadyNodesRO()
	if err != nil {
		return nil, err
	}

	return filterSchedulableNodes(nodes), nil
}

func (s *DataStore) ListReadyNodesContainingEngineImageRO(image string) (map[string]*longhorn.Node, error) {
	ei, err := s.GetEngineImageRO(types.GetEngineImageChecksumName(image))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get engine image %v", image)
	}
	if ei.Status.State != longhorn.EngineImageStateDeployed && ei.Status.State != longhorn.EngineImageStateDeploying {
		return map[string]*longhorn.Node{}, nil
	}
	nodes, err := s.ListNodesContainingEngineImageRO(ei)
	if err != nil {
		return nil, err
	}
	readyNodes := filterReadyNodes(nodes)
	return readyNodes, nil
}

// GetReadyNodeDiskForBackingImage a list of all Node the in the given namespace and
// returns the first Node && the first Disk of the Node marked with condition ready and allow scheduling
func (s *DataStore) GetReadyNodeDiskForBackingImage(backingImage *longhorn.BackingImage, dataEngine longhorn.DataEngineType, nodeList []*longhorn.Node) (*longhorn.Node, string, error) {
	logrus.Info("Preparing to find a random ready node disk")
	if nodeList == nil {
		nodes, err := s.ListNodesRO()
		if err != nil {
			return nil, "", errors.Wrapf(err, "failed to get random ready node disk")
		}
		nodeList = nodes
	}

	allowEmptyNodeSelectorVolume, err := s.GetSettingAsBool(types.SettingNameAllowEmptyNodeSelectorVolume)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyNodeSelectorVolume)
	}

	allowEmptyDiskSelectorVolume, err := s.GetSettingAsBool(types.SettingNameAllowEmptyDiskSelectorVolume)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyDiskSelectorVolume)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(nodeList), func(i, j int) { nodeList[i], nodeList[j] = nodeList[j], nodeList[i] })
	for _, node := range nodeList {
		if !types.IsSelectorsInTags(node.Spec.Tags, backingImage.Spec.NodeSelector, allowEmptyNodeSelectorVolume) {
			continue
		}

		if !node.Spec.AllowScheduling {
			continue
		}
		if types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
			continue
		}
		for diskName, diskStatus := range node.Status.DiskStatus {
			diskSpec, exists := node.Spec.Disks[diskName]
			if !exists {
				continue
			}
			if !types.IsSelectorsInTags(diskSpec.Tags, backingImage.Spec.DiskSelector, allowEmptyDiskSelectorVolume) {
				continue
			}
			if types.IsDataEngineV2(dataEngine) {
				if diskSpec.Type != longhorn.DiskTypeBlock {
					continue
				}
			} else {
				if diskSpec.Type != longhorn.DiskTypeFilesystem {
					continue
				}
			}
			if _, exists := backingImage.Spec.DiskFileSpecMap[diskStatus.DiskUUID]; exists {
				continue
			}
			if !diskSpec.AllowScheduling {
				continue
			}
			if types.GetCondition(diskStatus.Conditions, longhorn.DiskConditionTypeSchedulable).Status != longhorn.ConditionStatusTrue {
				continue
			}

			return node.DeepCopy(), diskName, nil
		}
	}

	return nil, "", fmt.Errorf("unable to get a ready node disk")
}

// RemoveFinalizerForNode will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForNode(obj *longhorn.Node) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Nodes(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for node %v", obj.Name)
	}
	return nil
}

func (s *DataStore) IsNodeDownOrDeletedOrMissingManager(name string) (bool, error) {
	if name == "" {
		return false, errors.New("no node name provided to IsNodeDownOrDeletedOrMissingManager")
	}
	node, err := s.GetNodeRO(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	cond := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
	if cond.Status == longhorn.ConditionStatusFalse &&
		(cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeGone) ||
			cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeNotReady) ||
			cond.Reason == string(longhorn.NodeConditionReasonManagerPodMissing)) {
		return true, nil
	}
	return false, nil
}

// IsNodeDownOrDeleted gets Node for the given name and namespace and checks
// if the Node condition is gone or not ready
func (s *DataStore) IsNodeDownOrDeleted(name string) (bool, error) {
	if name == "" {
		return false, errors.New("no node name provided to check node down or deleted")
	}
	node, err := s.GetNodeRO(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	cond := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
	if cond.Status == longhorn.ConditionStatusFalse &&
		(cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeGone) ||
			cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeNotReady)) {
		return true, nil
	}
	return false, nil
}

// IsNodeDelinquent checks an early-warning condition of Lease expiration
// that is of interest to share-manager types.
func (s *DataStore) IsNodeDelinquent(nodeName string, volumeName string) (bool, error) {
	if nodeName == "" || volumeName == "" {
		return false, nil
	}

	isRWX, err := s.IsRegularRWXVolume(volumeName)
	if err != nil {
		return false, err
	}
	if !isRWX {
		return false, nil
	}

	isDelinquent, delinquentNode, err := s.IsRWXVolumeDelinquent(volumeName)
	if err != nil {
		return false, err
	}
	return isDelinquent && delinquentNode == nodeName, nil
}

// IsNodeDownOrDeletedOrDelinquent gets Node for the given name and checks
// if the Node condition is gone or not ready or, if we are asking on behalf
// of an RWX-related resource, delinquent for that resource's volume.
func (s *DataStore) IsNodeDownOrDeletedOrDelinquent(nodeName string, volumeName string) (bool, error) {
	if nodeName == "" {
		return false, errors.New("no node name provided to check node down or deleted or delinquent")
	}
	node, err := s.GetNodeRO(nodeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	cond := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
	if cond.Status == longhorn.ConditionStatusFalse &&
		(cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeGone) ||
			cond.Reason == string(longhorn.NodeConditionReasonKubernetesNodeNotReady)) {
		return true, nil
	}

	return s.IsNodeDelinquent(nodeName, volumeName)
}

// IsNodeDeleted checks whether the node does not exist by passing in the node name
func (s *DataStore) IsNodeDeleted(name string) (bool, error) {
	if name == "" {
		return false, errors.New("no node name provided to check node down or deleted")
	}

	node, err := s.GetNodeRO(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	cond := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)

	return cond.Status == longhorn.ConditionStatusFalse && cond.Reason == longhorn.NodeConditionReasonKubernetesNodeGone, nil
}

func (s *DataStore) IsNodeSchedulable(name string) bool {
	node, err := s.GetNodeRO(name)
	if err != nil {
		return false
	}
	nodeSchedulableCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable)
	return nodeSchedulableCondition.Status == longhorn.ConditionStatusTrue
}

func getNodeSelector(nodeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.LonghornNodeKey: nodeName,
		},
	})
}

func getLonghornNodeSelector(nodeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.GetLonghornLabelKey(types.LonghornLabelNode): nodeName,
		},
	})
}

// ListReplicasByNode gets a map of Replicas on the node Name for the given namespace.
func (s *DataStore) ListReplicasByNode(name string) (map[string]*longhorn.Replica, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.listReplicas(nodeSelector)
}

// ListReplicasByDiskUUID gets a list of Replicas on a specific disk the given namespace.
func (s *DataStore) ListReplicasByDiskUUID(uuid string) (map[string]*longhorn.Replica, error) {
	diskSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.LonghornDiskUUIDKey: uuid,
		},
	})
	if err != nil {
		return nil, err
	}
	return s.listReplicas(diskSelector)
}

func getBackingImageSelector(backingImageName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			types.GetLonghornLabelKey(types.LonghornLabelBackingImage): backingImageName,
		},
	})
}

// ListReplicasByBackingImage gets a list of Replicas using a specific backing image the given namespace.
func (s *DataStore) ListReplicasByBackingImage(backingImageName string) ([]*longhorn.Replica, error) {
	backingImageSelector, err := getBackingImageSelector(backingImageName)
	if err != nil {
		return nil, err
	}
	return s.replicaLister.Replicas(s.namespace).List(backingImageSelector)
}

// ListReplicasByNodeRO returns a list of all Replicas on node Name for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListReplicasByNodeRO(name string) ([]*longhorn.Replica, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.replicaLister.Replicas(s.namespace).List(nodeSelector)
}

func labelNode(nodeID string, obj k8sruntime.Object) error {
	// fix longhornnode label for object
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.LonghornNodeKey] = nodeID
	metadata.SetLabels(labels)
	return nil
}

func labelDiskUUID(diskUUID string, obj k8sruntime.Object) error {
	// fix longhorndiskuuid label for object
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.LonghornDiskUUIDKey] = diskUUID
	metadata.SetLabels(labels)
	return nil
}

// labelLonghornNode fixes the new label `longhorn.io/node` for object.
// It can replace func `labelNode` if the old label `longhornnode` is
// deprecated.
func labelLonghornNode(nodeID string, obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetLonghornLabelKey(types.LonghornLabelNode)] = nodeID
	metadata.SetLabels(labels)
	return nil
}

// labelLonghornDiskUUID fixes the new label `longhorn.io/disk-uuid` for
// object. It can replace func `labelDiskUUID` if the old label
// `longhorndiskuuid` is deprecated.
func labelLonghornDiskUUID(diskUUID string, obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetLonghornLabelKey(types.LonghornLabelDiskUUID)] = diskUUID
	metadata.SetLabels(labels)
	return nil
}

func labelBackingImage(backingImageName string, obj k8sruntime.Object) error {
	// fix longhorn.io/backing-image label for object
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetLonghornLabelKey(types.LonghornLabelBackingImage)] = backingImageName
	metadata.SetLabels(labels)
	return nil
}

func labelBackupVolume(backupVolumeName string, obj k8sruntime.Object) error {
	// fix longhorn.io/backup-volume label for object
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.LonghornLabelBackupVolume] = backupVolumeName
	metadata.SetLabels(labels)
	return nil
}

// labelLonghornDeleteCustomResourceOnly labels the object with the label `longhorn.io/delete-custom-resource-only: true`
func labelLonghornDeleteCustomResourceOnly(obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetLonghornLabelKey(types.DeleteCustomResourceOnly)] = "true"
	metadata.SetLabels(labels)
	return nil
}

// IsLabelLonghornDeleteCustomResourceOnlyExisting check if the object label `longhorn.io/delete-custom-resource-only` exists
func IsLabelLonghornDeleteCustomResourceOnlyExisting(obj k8sruntime.Object) (bool, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		return false, nil
	}
	_, ok := labels[types.GetLonghornLabelKey(types.DeleteCustomResourceOnly)]
	return ok, nil
}

// AddBackupVolumeDeleteCustomResourceOnlyLabel adds the label `longhorn.io/delete-custom-resource-only: true` to the BackupVolume
func AddBackupVolumeDeleteCustomResourceOnlyLabel(ds *DataStore, backupVolumeName string) error {
	bv, err := ds.GetBackupVolume(backupVolumeName)
	if err != nil {
		return err
	}
	if exists, err := IsLabelLonghornDeleteCustomResourceOnlyExisting(bv); err != nil {
		return err
	} else if exists {
		return nil
	}
	if err := labelLonghornDeleteCustomResourceOnly(bv); err != nil {
		return err
	}
	if _, err = ds.UpdateBackupVolume(bv); err != nil {
		return err
	}

	return nil
}

// AddBackupDeleteCustomResourceOnlyLabel adds the label `longhorn.io/delete-custom-resource-only: true` to the Backup
func AddBackupDeleteCustomResourceOnlyLabel(ds *DataStore, backupName string) error {
	backup, err := ds.GetBackup(backupName)
	if err != nil {
		return err
	}
	if exists, err := IsLabelLonghornDeleteCustomResourceOnlyExisting(backup); err != nil {
		return err
	} else if exists {
		return nil
	}
	if err := labelLonghornDeleteCustomResourceOnly(backup); err != nil {
		return err
	}
	if _, err = ds.UpdateBackup(backup); err != nil {
		return err
	}

	return nil
}

// AddBackupBackingImageDeleteCustomResourceOnlyLabel adds the label `longhorn.io/delete-custom-resource-only: true` to the BackupBackingImage
func AddBackupBackingImageDeleteCustomResourceOnlyLabel(ds *DataStore, backupBackingImageName string) error {
	bbi, err := ds.GetBackupBackingImage(backupBackingImageName)
	if err != nil {
		return err
	}
	if exists, err := IsLabelLonghornDeleteCustomResourceOnlyExisting(bbi); err != nil {
		return err
	} else if exists {
		return nil
	}
	if err := labelLonghornDeleteCustomResourceOnly(bbi); err != nil {
		return err
	}
	if _, err = ds.UpdateBackupBackingImage(bbi); err != nil {
		return err
	}

	return nil
}

// AddSystemBackupDeleteCustomResourceOnlyLabel adds the label `longhorn.io/delete-custom-resource-only: true` to the SystemBackup
func AddSystemBackupDeleteCustomResourceOnlyLabel(ds *DataStore, systemBackupName string) error {
	sb, err := ds.GetSystemBackup(systemBackupName)
	if err != nil {
		return err
	}
	if exists, err := IsLabelLonghornDeleteCustomResourceOnlyExisting(sb); err != nil {
		return err
	} else if exists {
		return nil
	}
	if err := labelLonghornDeleteCustomResourceOnly(sb); err != nil {
		return err
	}
	if _, err = ds.UpdateSystemBackup(sb); err != nil {
		return err
	}

	return nil
}

func FixupRecurringJob(v *longhorn.Volume) error {
	if err := labelRecurringJobDefault(v); err != nil {
		return err
	}
	return nil
}

func labelRecurringJobDefault(obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	jobPrefix := fmt.Sprintf(types.LonghornLabelRecurringJobKeyPrefixFmt, types.LonghornLabelRecurringJob)
	groupPrefix := fmt.Sprintf(types.LonghornLabelRecurringJobKeyPrefixFmt, types.LonghornLabelRecurringJobGroup)

	jobLabels := []string{}
	for label := range labels {
		if !strings.HasPrefix(label, jobPrefix) &&
			!strings.HasPrefix(label, groupPrefix) {
			continue
		}
		jobLabels = append(jobLabels, label)
	}

	defaultLabel := types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJobGroup, longhorn.RecurringJobGroupDefault)
	jobLabelCount := len(jobLabels)
	if jobLabelCount == 0 {
		labels[defaultLabel] = types.LonghornLabelValueEnabled
	}
	metadata.SetLabels(labels)
	return nil
}

// GetOwnerReferencesForNode returns a list contains a single OwnerReference
// for the given Node ID and name
func GetOwnerReferencesForNode(node *longhorn.Node) []metav1.OwnerReference {
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion:         longhorn.SchemeGroupVersion.String(),
			Kind:               types.LonghornKindNode,
			Name:               node.Name,
			UID:                node.UID,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// GetSettingAsInt gets the setting for the given name, returns as integer
// Returns error if the definition type is not integer
func (s *DataStore) GetSettingAsInt(settingName types.SettingName) (int64, error) {
	definition, ok := types.GetSettingDefinition(settingName)
	if !ok {
		return -1, fmt.Errorf("setting %v is not supported", settingName)
	}
	settings, err := s.GetSettingWithAutoFillingRO(settingName)
	if err != nil {
		return -1, err
	}
	value := settings.Value

	if definition.Type == types.SettingTypeInt {
		result, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return -1, err
		}
		return result, nil
	}

	return -1, fmt.Errorf("the %v setting value couldn't change to integer, value is %v ", string(settingName), value)
}

// GetSettingAsBool gets the setting for the given name, returns as boolean
// Returns error if the definition type is not boolean
func (s *DataStore) GetSettingAsBool(settingName types.SettingName) (bool, error) {
	definition, ok := types.GetSettingDefinition(settingName)
	if !ok {
		return false, fmt.Errorf("setting %v is not supported", settingName)
	}
	settings, err := s.GetSettingWithAutoFillingRO(settingName)
	if err != nil {
		return false, err
	}
	value := settings.Value

	if definition.Type == types.SettingTypeBool {
		result, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return result, nil
	}

	return false, fmt.Errorf("the %v setting value couldn't be converted to bool, value is %v ", string(settingName), value)
}

// GetSettingImagePullPolicy get the setting and return one of Kubernetes ImagePullPolicy definition
// Returns error if the ImagePullPolicy is invalid
func (s *DataStore) GetSettingImagePullPolicy() (corev1.PullPolicy, error) {
	ipp, err := s.GetSettingWithAutoFillingRO(types.SettingNameSystemManagedPodsImagePullPolicy)
	if err != nil {
		return "", err
	}
	switch ipp.Value {
	case string(types.SystemManagedPodsImagePullPolicyNever):
		return corev1.PullNever, nil
	case string(types.SystemManagedPodsImagePullPolicyIfNotPresent):
		return corev1.PullIfNotPresent, nil
	case string(types.SystemManagedPodsImagePullPolicyAlways):
		return corev1.PullAlways, nil
	}
	return "", fmt.Errorf("invalid image pull policy %v", ipp.Value)
}

func (s *DataStore) GetSettingTaintToleration() ([]corev1.Toleration, error) {
	setting, err := s.GetSettingWithAutoFillingRO(types.SettingNameTaintToleration)
	if err != nil {
		return nil, err
	}
	tolerationList, err := types.UnmarshalTolerations(setting.Value)
	if err != nil {
		return nil, err
	}
	return tolerationList, nil
}

func (s *DataStore) GetSettingSystemManagedComponentsNodeSelector() (map[string]string, error) {
	setting, err := s.GetSettingWithAutoFillingRO(types.SettingNameSystemManagedComponentsNodeSelector)
	if err != nil {
		return nil, err
	}
	nodeSelector, err := types.UnmarshalNodeSelector(setting.Value)
	if err != nil {
		return nil, err
	}
	return nodeSelector, nil
}

// ResetMonitoringEngineStatus clean and update Engine status
func (s *DataStore) ResetMonitoringEngineStatus(e *longhorn.Engine) (*longhorn.Engine, error) {
	e.Status.Endpoint = ""
	e.Status.LastRestoredBackup = ""
	e.Status.ReplicaModeMap = nil
	e.Status.ReplicaTransitionTimeMap = nil
	e.Status.RestoreStatus = nil
	e.Status.PurgeStatus = nil
	e.Status.RebuildStatus = nil
	e.Status.LastExpansionFailedAt = ""
	e.Status.LastExpansionError = ""
	ret, err := s.UpdateEngineStatus(e)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reset engine status for %v", e.Name)
	}
	return ret, nil
}

// DeleteNode deletes Node for the given name and namespace
func (s *DataStore) DeleteNode(name string) error {
	return s.lhClient.LonghornV1beta2().Nodes(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ListEnginesByNodeRO returns a list of all Engines on node Name for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListEnginesByNodeRO(name string) ([]*longhorn.Engine, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}

	engineList, err := s.engineLister.Engines(s.namespace).List(nodeSelector)
	if err != nil {
		return nil, err
	}

	return engineList, nil
}

// GetOwnerReferencesForInstanceManager returns OwnerReference for the given
// instance Manager name and UID
func GetOwnerReferencesForInstanceManager(im *longhorn.InstanceManager) []metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindInstanceManager,
			Name:       im.Name,
			UID:        im.UID,
			// This field is needed so that `kubectl drain` can work without --force flag
			// See https://github.com/longhorn/longhorn/issues/1286#issuecomment-623283028 for more details
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// CreateInstanceManager creates a Longhorn InstanceManager resource and
// verifies creation
func (s *DataStore) CreateInstanceManager(im *longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	ret, err := s.lhClient.LonghornV1beta2().InstanceManagers(s.namespace).Create(context.TODO(), im, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "instance manager", func(name string) (k8sruntime.Object, error) {
		return s.GetInstanceManagerRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.InstanceManager)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for instance manager")
	}

	return ret.DeepCopy(), nil
}

// DeleteInstanceManager deletes the InstanceManager.
// The dependents will be deleted in the foreground
func (s *DataStore) DeleteInstanceManager(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().InstanceManagers(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

func (s *DataStore) GetInstanceManagerRO(name string) (*longhorn.InstanceManager, error) {
	return s.instanceManagerLister.InstanceManagers(s.namespace).Get(name)
}

// GetInstanceManager gets the InstanceManager for the given name and namespace.
// Returns new InstanceManager object
func (s *DataStore) GetInstanceManager(name string) (*longhorn.InstanceManager, error) {
	resultRO, err := s.GetInstanceManagerRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// GetDefaultInstanceManagerByNodeRO returns the given node's engine InstanceManager
// that is using the default instance manager image.
// The object is the direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copy.
func (s *DataStore) GetDefaultInstanceManagerByNodeRO(name string, dataEngine longhorn.DataEngineType) (*longhorn.InstanceManager, error) {
	defaultInstanceManagerImage, err := s.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
	if err != nil {
		return nil, err
	}

	instanceManagers, err := s.ListInstanceManagersBySelectorRO(name, defaultInstanceManagerImage, longhorn.InstanceManagerTypeAllInOne, dataEngine)
	if err != nil {
		return nil, err
	}

	instanceManager := &longhorn.InstanceManager{}
	for _, im := range instanceManagers {
		instanceManager = im

		if len(instanceManagers) != 1 {
			logrus.Debugf("Found more than 1 %v instance manager with %v on %v, use %v", longhorn.InstanceManagerTypeEngine, defaultInstanceManagerImage, name, instanceManager.Name)
			break
		}
	}

	return instanceManager, nil
}

// CheckInstanceManagerType checks and returns InstanceManager labels type
// Returns error if the InstanceManager type is not engine or replica
func CheckInstanceManagerType(im *longhorn.InstanceManager) (longhorn.InstanceManagerType, error) {
	imTypeLabelkey := types.GetLonghornLabelKey(types.LonghornLabelInstanceManagerType)
	imType, exist := im.Labels[imTypeLabelkey]
	if !exist {
		return longhorn.InstanceManagerType(""), fmt.Errorf("no label %v in instance manager %v", imTypeLabelkey, im.Name)
	}

	switch imType {
	case string(longhorn.InstanceManagerTypeEngine):
		return longhorn.InstanceManagerTypeEngine, nil
	case string(longhorn.InstanceManagerTypeReplica):
		return longhorn.InstanceManagerTypeReplica, nil
	case string(longhorn.InstanceManagerTypeAllInOne):
		return longhorn.InstanceManagerTypeAllInOne, nil
	}

	return longhorn.InstanceManagerType(""), fmt.Errorf("unknown type %v for instance manager %v", imType, im.Name)
}

// CheckInstanceManagersReadiness checks if the InstanceManager is running on the
// specified set of nodes.
// There is one running instance manager for v2 data engine on each node, so the function
// find one running instance manager instead.
//
// Parameters:
//   - dataEngine: the data engine type.
//   - nodes: the list of node names.
//
// Returns:
// - Boolean indicating if all InstanceManagers are running on the nodes.
// - Error for any errors encountered.
func (s *DataStore) CheckInstanceManagersReadiness(dataEngine longhorn.DataEngineType, nodes ...string) (isReady bool, err error) {
	for _, node := range nodes {
		var instanceManager *longhorn.InstanceManager

		if types.IsDataEngineV2(dataEngine) {
			instanceManager, err = s.GetRunningInstanceManagerByNodeRO(node, dataEngine)
		} else {
			instanceManager, err = s.GetDefaultInstanceManagerByNodeRO(node, dataEngine)
		}
		if err != nil {
			return false, err
		}

		isRunning := instanceManager.Status.CurrentState == longhorn.InstanceManagerStateRunning
		notDeleted := instanceManager.DeletionTimestamp == nil
		if !isRunning && notDeleted {
			logrus.WithFields(
				logrus.Fields{
					"currentState":    instanceManager.Status.CurrentState,
					"dataEngine":      dataEngine,
					"instanceManager": instanceManager.Name,
					"node":            node,
				},
			).Error("CheckInstanceManagersReadiness: instance manager is not running")
			return false, nil
		}
	}

	return true, nil
}

// ListInstanceManagersBySelectorRO gets a list of InstanceManager by labels for
// the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies.
func (s *DataStore) ListInstanceManagersBySelectorRO(node, imImage string, imType longhorn.InstanceManagerType, dataEngine longhorn.DataEngineType) (map[string]*longhorn.InstanceManager, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetInstanceManagerLabels(node, imImage, imType, dataEngine),
	})
	if err != nil {
		return nil, err
	}

	imList, err := s.instanceManagerLister.InstanceManagers(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	imMap := make(map[string]*longhorn.InstanceManager, len(imList))
	for _, imRO := range imList {
		imMap[imRO.Name] = imRO
	}

	return imMap, nil
}

// GetInstanceManagerByInstance returns an InstanceManager for a given object,
// or an error if more than one InstanceManager is found.
func (s *DataStore) GetInstanceManagerByInstance(obj interface{}) (*longhorn.InstanceManager, error) {
	im, err := s.GetInstanceManagerByInstanceRO(obj)
	if err != nil {
		return nil, err
	}
	return im.DeepCopy(), nil
}

func (s *DataStore) GetInstanceManagerByInstanceRO(obj interface{}) (*longhorn.InstanceManager, error) {
	var (
		name       string // name of the object
		nodeID     string
		dataEngine longhorn.DataEngineType
	)

	switch obj := obj.(type) {
	case *longhorn.Engine:
		name = obj.Name
		dataEngine = obj.Spec.DataEngine
		nodeID = obj.Spec.NodeID
	case *longhorn.Replica:
		name = obj.Name
		dataEngine = obj.Spec.DataEngine
		nodeID = obj.Spec.NodeID
	default:
		return nil, fmt.Errorf("unknown type for GetInstanceManagerByInstance, %+v", obj)
	}
	if nodeID == "" {
		return nil, fmt.Errorf("invalid request for GetInstanceManagerByInstance: no NodeID specified for instance %v", name)
	}

	imMap, err := s.listInstanceManagers(nodeID, dataEngine)
	if err != nil {
		return nil, err
	}

	return filterInstanceManagers(nodeID, dataEngine, imMap)
}

func (s *DataStore) listInstanceManagers(nodeID string, dataEngine longhorn.DataEngineType) (imMap map[string]*longhorn.InstanceManager, err error) {
	if types.IsDataEngineV2(dataEngine) {
		// Because there is only one active instance manager image for v2 data engine,
		// use spec.image as the instance manager image instead.
		imMap, err = s.ListInstanceManagersByNodeRO(nodeID, longhorn.InstanceManagerTypeAllInOne, dataEngine)
		if err != nil {
			return nil, fmt.Errorf("failed to list all instance managers for node %v: %w", nodeID, err)
		}
	} else {
		// Always use the default instance manager image for v1 data engine
		instanceManagerImage, err := s.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get default instance manager image")
		}

		imMap, err = s.ListInstanceManagersBySelectorRO(nodeID, instanceManagerImage, longhorn.InstanceManagerTypeAllInOne, dataEngine)
		if err != nil {
			return nil, fmt.Errorf("failed to list instance managers with image %v for node %v: %w", instanceManagerImage, nodeID, err)
		}
	}

	return imMap, nil
}

func filterInstanceManagers(nodeID string, dataEngine longhorn.DataEngineType, imMap map[string]*longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	if len(imMap) == 0 {
		return nil, fmt.Errorf("no instance manager found for node %v", nodeID)
	}

	if len(imMap) == 1 {
		for _, im := range imMap {
			return im, nil
		}
	}

	// Because there is only one active instance manager image for v2 data engine,
	// find a running instance manager for v2 data engine.
	if types.IsDataEngineV2(dataEngine) {
		return findRunningInstanceManager(imMap)
	}

	return nil, fmt.Errorf("ambiguous instance manager selection for node %v", nodeID)
}

func findRunningInstanceManager(imMap map[string]*longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	var runningIM *longhorn.InstanceManager
	for _, im := range imMap {
		if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
			if runningIM != nil {
				return nil, fmt.Errorf("found more than one running instance manager")
			}
			runningIM = im
		}
	}
	if runningIM == nil {
		return nil, fmt.Errorf("no running instance manager found")
	}
	return runningIM, nil
}

func (s *DataStore) ListInstanceManagersByNodeRO(node string, imType longhorn.InstanceManagerType, dataEngine longhorn.DataEngineType) (map[string]*longhorn.InstanceManager, error) {
	return s.ListInstanceManagersBySelectorRO(node, "", imType, dataEngine)
}

// ListInstanceManagers gets a list of InstanceManagers for the given namespace.
// Returns a new InstanceManager object
func (s *DataStore) ListInstanceManagers() (map[string]*longhorn.InstanceManager, error) {
	itemMap := map[string]*longhorn.InstanceManager{}

	list, err := s.instanceManagerLister.InstanceManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) ListInstanceManagersRO() (map[string]*longhorn.InstanceManager, error) {
	imList, err := s.instanceManagerLister.InstanceManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	imMap := make(map[string]*longhorn.InstanceManager, len(imList))
	for _, imRO := range imList {
		imMap[imRO.Name] = imRO
	}
	return imMap, nil
}

// UpdateInstanceManager updates Longhorn InstanceManager resource and verifies update
func (s *DataStore) UpdateInstanceManager(im *longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().InstanceManagers(s.namespace).Update(context.TODO(), im, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(im.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetInstanceManagerRO(name)
	})
	return obj, nil
}

// UpdateInstanceManagerStatus updates Longhorn InstanceManager resource status
// and verifies update
func (s *DataStore) UpdateInstanceManagerStatus(im *longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().InstanceManagers(s.namespace).UpdateStatus(context.TODO(), im, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(im.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetInstanceManagerRO(name)
	})
	return obj, nil
}

func verifyCreation(name, kind string, getMethod func(name string) (k8sruntime.Object, error)) (k8sruntime.Object, error) {
	// WORKAROUND: The immedidate read after object's creation can fail.
	// See https://github.com/longhorn/longhorn/issues/133
	var (
		ret k8sruntime.Object
		err error
	)
	for i := 0; i < VerificationRetryCounts; i++ {
		if ret, err = getMethod(name); err == nil {
			break
		}
		if !ErrorIsNotFound(err) {
			break
		}
		time.Sleep(VerificationRetryInterval)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to verify the existence of newly created %s %s", kind, name)
	}
	return ret, nil
}

func verifyUpdate(name string, obj k8sruntime.Object, getMethod func(name string) (k8sruntime.Object, error)) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		logrus.WithError(err).Errorf("BUG: datastore: cannot verify update for %v (%+v) because cannot get accessor", name, obj)
		return
	}
	minimalResourceVersion := accessor.GetResourceVersion()
	verified := false
	for i := 0; i < VerificationRetryCounts; i++ {
		ret, err := getMethod(name)
		if err != nil {
			logrus.WithError(err).Errorf("datastore: failed to get updated object %v", name)
			return
		}
		accessor, err := meta.Accessor(ret)
		if err != nil {
			logrus.WithError(err).Errorf("BUG: datastore: cannot verify update for %v because cannot get accessor for updated object", name)
			return
		}
		if resourceVersionAtLeast(accessor.GetResourceVersion(), minimalResourceVersion) {
			verified = true
			break
		}
		time.Sleep(VerificationRetryInterval)
	}
	if !verified {
		logrus.Errorf("Unable to verify the update of %s", name)
	}
}

// resourceVersionAtLeast depends on the Kubernetes internal resource version implementation
// See https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
func resourceVersionAtLeast(curr, min string) bool {
	// skip unit testing code
	if curr == "" || min == "" {
		return true
	}
	currVersion, err := strconv.ParseInt(curr, 10, 64)
	if err != nil {
		logrus.WithError(err).Errorf("datastore: failed to parse current resource version %v", curr)
		return false
	}
	minVersion, err := strconv.ParseInt(min, 10, 64)
	if err != nil {
		logrus.WithError(err).Errorf("datastore: failed to parse minimal resource version %v", min)
		return false
	}
	return currVersion >= minVersion
}

// GetEngineImageCLIAPIVersion get engine image for the given name and returns the
// CLIAPIVersion
func (s *DataStore) GetEngineImageCLIAPIVersion(imageName string) (int, error) {
	if imageName == "" {
		return -1, fmt.Errorf("cannot check the CLI API Version based on empty image name")
	}
	ei, err := s.GetEngineImageRO(types.GetEngineImageChecksumName(imageName))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get engine image object based on image name %v", imageName)
	}

	return ei.Status.CLIAPIVersion, nil
}

// GetDataEngineImageCLIAPIVersion get engine or instance manager image for the given name and returns the CLIAPIVersion
func (s *DataStore) GetDataEngineImageCLIAPIVersion(imageName string, dataEngine longhorn.DataEngineType) (int, error) {
	if imageName == "" {
		return -1, fmt.Errorf("cannot check the CLI API Version based on empty image name")
	}

	if types.IsDataEngineV2(dataEngine) {
		return 0, nil
	}

	ei, err := s.GetEngineImageRO(types.GetEngineImageChecksumName(imageName))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get engine image object based on image name %v", imageName)
	}

	return ei.Status.CLIAPIVersion, nil
}

// GetOwnerReferencesForShareManager returns OwnerReference for the given share manager name and UID
func GetOwnerReferencesForShareManager(sm *longhorn.ShareManager, isController bool) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindShareManager,
			Name:       sm.Name,
			UID:        sm.UID,
			Controller: &isController,
		},
	}
}

// CreateShareManager creates a Longhorn ShareManager resource and
// verifies creation
func (s *DataStore) CreateShareManager(sm *longhorn.ShareManager) (*longhorn.ShareManager, error) {
	ret, err := s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).Create(context.TODO(), sm, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "share manager", func(name string) (k8sruntime.Object, error) {
		return s.getShareManagerRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.ShareManager)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for share manager")
	}

	return ret.DeepCopy(), nil
}

// UpdateShareManager updates Longhorn ShareManager resource and verifies update
func (s *DataStore) UpdateShareManager(sm *longhorn.ShareManager) (*longhorn.ShareManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).Update(context.TODO(), sm, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(sm.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getShareManagerRO(name)
	})
	return obj, nil
}

// UpdateShareManagerStatus updates Longhorn ShareManager resource status and verifies update
func (s *DataStore) UpdateShareManagerStatus(sm *longhorn.ShareManager) (*longhorn.ShareManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).UpdateStatus(context.TODO(), sm, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(sm.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getShareManagerRO(name)
	})
	return obj, nil
}

// DeleteShareManager won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteShareManager(name string) error {
	return s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForShareManager will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForShareManager(obj *longhorn.ShareManager) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	if _, err := s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{}); err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for share manager %v", obj.Name)
	}
	return nil
}

func (s *DataStore) getShareManagerRO(name string) (*longhorn.ShareManager, error) {
	return s.shareManagerLister.ShareManagers(s.namespace).Get(name)
}

// GetShareManager gets the ShareManager for the given name and namespace.
// Returns a mutable ShareManager object
func (s *DataStore) GetShareManager(name string) (*longhorn.ShareManager, error) {
	result, err := s.getShareManagerRO(name)
	if err != nil {
		return nil, err
	}
	return result.DeepCopy(), nil
}

// ListShareManagers returns a map of ShareManagers indexed by name
func (s *DataStore) ListShareManagers() (map[string]*longhorn.ShareManager, error) {
	itemMap := map[string]*longhorn.ShareManager{}

	list, err := s.shareManagerLister.ShareManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) ListShareManagersRO() ([]*longhorn.ShareManager, error) {
	return s.shareManagerLister.ShareManagers(s.namespace).List(labels.Everything())
}

// CreateOrUpdateDefaultBackupTarget updates the default backup target from the ConfigMap longhorn-default-resource
func (s *DataStore) CreateOrUpdateDefaultBackupTarget() error {
	defaultBackupStoreCM, err := s.GetConfigMapRO(s.namespace, types.DefaultDefaultResourceConfigMapName)
	if err != nil {
		return err
	}

	defaultBackupStoreYAMLData := []byte(defaultBackupStoreCM.Data[types.DefaultResourceYAMLFileName])

	defaultBackupStoreParameters, err := util.GetDataContentFromYAML(defaultBackupStoreYAMLData)
	if err != nil {
		return err
	}

	if err := s.syncDefaultBackupTargetResourceWithCustomizedParameters(defaultBackupStoreParameters); err != nil {
		return err
	}
	return nil
}

func (s *DataStore) syncDefaultBackupTargetResourceWithCustomizedParameters(customizedDefaultBackupStoreParameters map[string]string) error {
	backupTarget := &longhorn.BackupTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: types.DefaultBackupTargetName,
		},
		Spec: longhorn.BackupTargetSpec{
			PollInterval: metav1.Duration{Duration: types.DefaultBackupstorePollInterval},
		},
	}
	backupTargetURL, urlExists := customizedDefaultBackupStoreParameters[string(types.SettingNameBackupTarget)]
	backupTargetCredentialSecret, secretExists := customizedDefaultBackupStoreParameters[string(types.SettingNameBackupTargetCredentialSecret)]
	pollIntervalStr, pollInterExists := customizedDefaultBackupStoreParameters[string(types.SettingNameBackupstorePollInterval)]

	if backupTargetURL != "" {
		backupTarget.Spec.BackupTargetURL = backupTargetURL
	}
	if backupTargetCredentialSecret != "" {
		backupTarget.Spec.CredentialSecret = backupTargetCredentialSecret
	}

	if pollIntervalStr != "" {
		pollIntervalAsInt, err := strconv.ParseInt(pollIntervalStr, 10, 64)
		if err != nil {
			return err
		}
		backupTarget.Spec.PollInterval = metav1.Duration{Duration: time.Duration(pollIntervalAsInt) * time.Second}
	}

	existingBackupTarget, err := s.GetBackupTarget(types.DefaultBackupTargetName)
	if err != nil {
		if !ErrorIsNotFound(err) {
			return err
		}
		_, err = s.CreateBackupTarget(backupTarget)
		if apierrors.IsAlreadyExists(err) {
			return nil // Already exists, no need to return error
		}
		return err
	}

	// If the parameters are not in the longhorn-default-resource ConfigMap, use the existing values.
	if !urlExists {
		backupTarget.Spec.BackupTargetURL = existingBackupTarget.Spec.BackupTargetURL
	}
	if !secretExists {
		backupTarget.Spec.CredentialSecret = existingBackupTarget.Spec.CredentialSecret
	}
	if !pollInterExists {
		backupTarget.Spec.PollInterval = existingBackupTarget.Spec.PollInterval
	}
	syncTime := metav1.Time{Time: time.Now().UTC()}
	backupTarget.Spec.SyncRequestedAt = syncTime
	existingBackupTarget.Spec.SyncRequestedAt = syncTime

	if reflect.DeepEqual(existingBackupTarget.Spec, backupTarget.Spec) {
		return nil // No changes, no need to update
	}

	existingBackupTarget.Spec = backupTarget.Spec
	_, err = s.UpdateBackupTarget(existingBackupTarget)
	return err
}

// CreateBackupTarget creates a Longhorn BackupTargets CR and verifies creation
func (s *DataStore) CreateBackupTarget(backupTarget *longhorn.BackupTarget) (*longhorn.BackupTarget, error) {
	ret, err := s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Create(context.TODO(), backupTarget, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backup target", func(name string) (k8sruntime.Object, error) {
		return s.GetBackupTargetRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackupTarget)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for BackupTarget")
	}
	return ret.DeepCopy(), nil
}

// ListBackupTargetsRO returns all BackupTargets in the cluster
func (s *DataStore) ListBackupTargetsRO() (map[string]*longhorn.BackupTarget, error) {
	list, err := s.backupTargetLister.BackupTargets(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		logrus.Debug("No backup targets found in the cluster")
	}

	itemMap := map[string]*longhorn.BackupTarget{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

// ListBackupTargets returns an object contains all backup targets in the cluster BackupTargets CR
func (s *DataStore) ListBackupTargets() (map[string]*longhorn.BackupTarget, error) {
	list, err := s.backupTargetLister.BackupTargets(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupTarget{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// GetDefaultBackupTargetRO returns the BackupTarget for the default backup target
func (s *DataStore) GetDefaultBackupTargetRO() (*longhorn.BackupTarget, error) {
	return s.GetBackupTargetRO(types.DefaultBackupTargetName)
}

// GetBackupTargetRO returns the BackupTarget with the given backup target name in the cluster
func (s *DataStore) GetBackupTargetRO(backupTargetName string) (*longhorn.BackupTarget, error) {
	return s.backupTargetLister.BackupTargets(s.namespace).Get(backupTargetName)
}

// GetBackupTarget returns a copy of BackupTarget with the given backup target name in the cluster
func (s *DataStore) GetBackupTarget(name string) (*longhorn.BackupTarget, error) {
	resultRO, err := s.GetBackupTargetRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateBackupTarget updates the given Longhorn backup target in the cluster BackupTargets CR and verifies update
func (s *DataStore) UpdateBackupTarget(backupTarget *longhorn.BackupTarget) (*longhorn.BackupTarget, error) {
	if backupTarget.Annotations == nil {
		backupTarget.Annotations = make(map[string]string)
	}

	obj, err := s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Update(context.TODO(), backupTarget, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupTarget.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupTargetRO(name)
	})
	return obj, nil
}

// UpdateBackupTargetStatus updates the given Longhorn backup target in the cluster BackupTargets CR status and verifies update
func (s *DataStore) UpdateBackupTargetStatus(backupTarget *longhorn.BackupTarget) (*longhorn.BackupTarget, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).UpdateStatus(context.TODO(), backupTarget, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupTarget.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupTargetRO(name)
	})
	return obj, nil
}

// DeleteBackupTarget won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteBackupTarget(backupTargetName string) error {
	return s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Delete(context.TODO(), backupTargetName, metav1.DeleteOptions{})
}

// RemoveFinalizerForBackupTarget will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackupTarget(backupTarget *longhorn.BackupTarget) error {
	if !util.FinalizerExists(longhornFinalizerKey, backupTarget) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, backupTarget); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Update(context.TODO(), backupTarget, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if backupTarget.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backup target %s", backupTarget.Name)
	}
	return nil
}

// ValidateBackupTargetURL checks if the given backup target URL is already used by other backup targets
func (s *DataStore) ValidateBackupTargetURL(backupTargetName, backupTargetURL string) error {
	if backupTargetURL == "" {
		return nil
	}
	u, err := url.Parse(backupTargetURL)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %v as url", backupTargetURL)
	}

	if err := checkBackupTargetURLFormat(u); err != nil {
		return err
	}
	if err := s.validateBackupTargetURLExisting(backupTargetName, u); err != nil {
		return err
	}

	return nil
}

func checkBackupTargetURLFormat(u *url.URL) error {
	// Check whether have $ or , have been set in BackupTarget path
	regStr := `[\$\,]`
	if u.Scheme == "cifs" {
		// The $ in SMB/CIFS URIs means that the share is hidden.
		regStr = `[\,]`
	}

	reg := regexp.MustCompile(regStr)
	findStr := reg.FindAllString(u.Path, -1)
	if len(findStr) != 0 {
		return fmt.Errorf("url %s, contains %v", u.String(), strings.Join(findStr, " or "))
	}
	return nil
}

// validateBackupTargetURLExisting checks if the given backup target URL is already used by other backup targets
func (s *DataStore) validateBackupTargetURLExisting(backupTargetName string, u *url.URL) error {
	newBackupTargetPath, err := getBackupTargetPath(u)
	if err != nil {
		return err
	}

	bts, err := s.ListBackupTargetsRO()
	if err != nil {
		return err
	}
	for _, bt := range bts {
		existingURL, err := url.Parse(bt.Spec.BackupTargetURL)
		if err != nil {
			return errors.Wrapf(err, "failed to parse %v as url", bt.Spec.BackupTargetURL)
		}
		if existingURL.Scheme != u.Scheme {
			continue
		}

		oldBackupTargetPath, err := getBackupTargetPath(existingURL)
		if err != nil {
			return err
		}
		if oldBackupTargetPath == newBackupTargetPath && backupTargetName != bt.Name {
			return fmt.Errorf("url %s is the same to backup target %v", u.String(), bt.Name)
		}
	}

	return nil
}

func getBackupTargetPath(u *url.URL) (backupTargetPath string, err error) {
	switch u.Scheme {
	case types.BackupStoreTypeAZBlob, types.BackupStoreTypeS3:
		backupTargetPath = strings.ToLower(u.String())
	case types.BackupStoreTypeCIFS, types.BackupStoreTypeNFS:
		backupTargetPath = strings.ToLower(strings.TrimRight(u.Host+u.Path, "/"))
	default:
		return "", fmt.Errorf("url %s with the unsupported protocol %v", u.String(), u.Scheme)
	}

	return backupTargetPath, nil
}

// CreateBackupVolume creates a Longhorn BackupVolumes CR and verifies creation
func (s *DataStore) CreateBackupVolume(backupVolume *longhorn.BackupVolume) (*longhorn.BackupVolume, error) {
	ret, err := s.lhClient.LonghornV1beta2().BackupVolumes(s.namespace).Create(context.TODO(), backupVolume, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backup volume", func(name string) (k8sruntime.Object, error) {
		return s.GetBackupVolumeRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackupVolume)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for BackupVolume")
	}
	return ret.DeepCopy(), nil
}

// ListBackupVolumes returns an object contains all backup volumes in the cluster BackupVolumes CR
func (s *DataStore) ListBackupVolumes() (map[string]*longhorn.BackupVolume, error) {
	list, err := s.backupVolumeLister.BackupVolumes(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupVolume{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListBackupVolumesWithBackupTargetNameRO returns an object contains all backup volumes in the cluster BackupVolumes CR
// of the given backup target name
func (s *DataStore) ListBackupVolumesWithBackupTargetNameRO(backupTargetName string) (map[string]*longhorn.BackupVolume, error) {
	selector, err := getBackupTargetSelector(backupTargetName)
	if err != nil {
		return nil, err
	}

	list, err := s.backupVolumeLister.BackupVolumes(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupVolume{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

// ListBackupVolumesWithVolumeNameRO returns an object contains all backup volumes in the cluster BackupVolumes CR
// of the given volume name
func (s *DataStore) ListBackupVolumesWithVolumeNameRO(volumeName string) (map[string]*longhorn.BackupVolume, error) {
	selector, err := getBackupVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}

	list, err := s.backupVolumeLister.BackupVolumes(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupVolume{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

// GetBackupVolumeByBackupTargetAndVolumeRO returns a backup volume object using the given backup target and volume name in the cluster
func (s *DataStore) GetBackupVolumeByBackupTargetAndVolumeRO(backupTargetName, volumeName string) (*longhorn.BackupVolume, error) {
	if backupTargetName == "" || volumeName == "" {
		return nil, fmt.Errorf("backup target name and volume name cannot be empty")
	}
	selector, err := getBackupVolumeWithBackupTargetSelector(backupTargetName, volumeName)
	if err != nil {
		return nil, err
	}

	list, err := s.backupVolumeLister.BackupVolumes(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	if len(list) > 1 {
		return nil, fmt.Errorf("datastore: found more than one backup volume with backup target %v and volume %v", backupTargetName, volumeName)
	}

	for _, itemRO := range list {
		return itemRO, nil
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "backupVolumes"}, "")
}

// GetBackupVolumeByBackupTargetAndVolume returns a copy of BackupVolume with the given backup target and volume name in the cluster
func (s *DataStore) GetBackupVolumeByBackupTargetAndVolume(backupTargetName, volumeName string) (*longhorn.BackupVolume, error) {
	resultRO, err := s.GetBackupVolumeByBackupTargetAndVolumeRO(backupTargetName, volumeName)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func getBackupTargetSelector(backupTargetName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackupTargetLabels(backupTargetName),
	})
}

func getBackupVolumeSelector(volumeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackupVolumeLabels(volumeName),
	})
}

// GetBackupVolumeRO returns the BackupVolume with the given backup volume name in the cluster
func (s *DataStore) GetBackupVolumeRO(backupVolumeName string) (*longhorn.BackupVolume, error) {
	return s.backupVolumeLister.BackupVolumes(s.namespace).Get(backupVolumeName)
}

func getBackupVolumeWithBackupTargetSelector(backupTargetName, volumeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackupVolumeWithBackupTargetLabels(backupTargetName, volumeName),
	})
}

// GetBackupVolume returns a copy of BackupVolume with the given backup volume name in the cluster
func (s *DataStore) GetBackupVolume(name string) (*longhorn.BackupVolume, error) {
	resultRO, err := s.GetBackupVolumeRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateBackupVolume updates the given Longhorn backup volume in the cluster BackupVolume CR and verifies update
func (s *DataStore) UpdateBackupVolume(backupVolume *longhorn.BackupVolume) (*longhorn.BackupVolume, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackupVolumes(s.namespace).Update(context.TODO(), backupVolume, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupVolume.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupVolumeRO(name)
	})
	return obj, nil
}

// UpdateBackupVolumeStatus updates the given Longhorn backup volume in the cluster BackupVolumes CR status and verifies update
func (s *DataStore) UpdateBackupVolumeStatus(backupVolume *longhorn.BackupVolume) (*longhorn.BackupVolume, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackupVolumes(s.namespace).UpdateStatus(context.TODO(), backupVolume, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupVolume.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupVolumeRO(name)
	})
	return obj, nil
}

// DeleteBackupVolume won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteBackupVolume(backupVolumeName string) error {
	return s.lhClient.LonghornV1beta2().BackupVolumes(s.namespace).Delete(context.TODO(), backupVolumeName, metav1.DeleteOptions{})
}

// RemoveFinalizerForBackupVolume will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackupVolume(backupVolume *longhorn.BackupVolume) error {
	if !util.FinalizerExists(longhornFinalizerKey, backupVolume) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, backupVolume); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackupVolumes(s.namespace).Update(context.TODO(), backupVolume, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if backupVolume.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backup volume %s", backupVolume.Name)
	}
	return nil
}

// CreateBackup creates a Longhorn Backup CR and verifies creation
func (s *DataStore) CreateBackup(backup *longhorn.Backup, backupVolumeName string) (*longhorn.Backup, error) {
	if err := labelBackupVolume(backupVolumeName, backup); err != nil {
		return nil, err
	}
	ret, err := s.lhClient.LonghornV1beta2().Backups(s.namespace).Create(context.TODO(), backup, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backup", func(name string) (k8sruntime.Object, error) {
		return s.GetBackupRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Backup)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for Backup")
	}
	return ret.DeepCopy(), nil
}

func (s *DataStore) listBackupsWithVolNameRO(volName, btName string) ([]*longhorn.Backup, error) {
	selector, err := getBackupVolumeSelector(volName)
	if err != nil {
		return nil, err
	}
	if btName != "" {
		selector, err = getBackupVolumeWithBackupTargetSelector(btName, volName)
		if err != nil {
			return nil, err
		}
	}
	return s.backupLister.Backups(s.namespace).List(selector)
}

// ListBackupsWithVolumeNameRO returns an object contains all read-only backups in the cluster Backups CR
// of a given volume name and a optional backup target name
func (s *DataStore) ListBackupsWithVolumeNameRO(volumeName, backupTargetName string) (map[string]*longhorn.Backup, error) {
	list, err := s.listBackupsWithVolNameRO(volumeName, backupTargetName)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Backup{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

func (s *DataStore) listBackupsWithBTAndVolNameRO(btName, volName string) ([]*longhorn.Backup, error) {
	selector, err := getBackupVolumeWithBackupTargetSelector(btName, volName)
	if err != nil {
		return nil, err
	}
	return s.backupLister.Backups(s.namespace).List(selector)
}

// ListBackupsWithBackupTargetAndBackupVolumeRO returns an object contains all read-only backups in the cluster Backups CR
// of the given volume name and backup target name
func (s *DataStore) ListBackupsWithBackupTargetAndBackupVolumeRO(backupTargetName, volumeName string) (map[string]*longhorn.Backup, error) {
	list, err := s.listBackupsWithBTAndVolNameRO(backupTargetName, volumeName)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Backup{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

// ListBackupsWithVolumeNameAndBackupTarget returns an object contains all backups in the cluster Backups CR
// of the given volume name and backup target name
func (s *DataStore) ListBackupsWithBackupVolumeName(backupTargetName, volumeName string) (map[string]*longhorn.Backup, error) {
	list, err := s.listBackupsWithBTAndVolNameRO(backupTargetName, volumeName)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Backup{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListBackupsRO returns a list of all Backups for the given namespace
func (s *DataStore) ListBackupsRO() ([]*longhorn.Backup, error) {
	return s.backupLister.Backups(s.namespace).List(labels.Everything())
}

// ListBackups returns an object contains all backups in the cluster Backups CR
func (s *DataStore) ListBackups() (map[string]*longhorn.Backup, error) {
	list, err := s.backupLister.Backups(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Backup{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// GetBackupRO returns the Backup with the given backup name in the cluster
func (s *DataStore) GetBackupRO(backupName string) (*longhorn.Backup, error) {
	return s.backupLister.Backups(s.namespace).Get(backupName)
}

// GetBackup returns a copy of Backup with the given backup name in the cluster
func (s *DataStore) GetBackup(name string) (*longhorn.Backup, error) {
	resultRO, err := s.GetBackupRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateBackup updates the given Longhorn backup in the cluster Backup CR and verifies update
func (s *DataStore) UpdateBackup(backup *longhorn.Backup) (*longhorn.Backup, error) {
	obj, err := s.lhClient.LonghornV1beta2().Backups(s.namespace).Update(context.TODO(), backup, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backup.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupRO(name)
	})
	return obj, nil
}

// UpdateBackupStatus updates the given Longhorn backup status in the cluster Backups CR status and verifies update
func (s *DataStore) UpdateBackupStatus(backup *longhorn.Backup) (*longhorn.Backup, error) {
	obj, err := s.lhClient.LonghornV1beta2().Backups(s.namespace).UpdateStatus(context.TODO(), backup, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backup.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupRO(name)
	})
	return obj, nil
}

// DeleteBackup won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteBackup(backupName string) error {
	return s.lhClient.LonghornV1beta2().Backups(s.namespace).Delete(context.TODO(), backupName, metav1.DeleteOptions{})
}

// DeleteAllBackupsForBackupVolumeWithBackupTarget won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteAllBackupsForBackupVolumeWithBackupTarget(backuptargetName, volumeName string) error {
	return s.lhClient.LonghornV1beta2().Backups(s.namespace).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: labels.Set(types.GetBackupVolumeWithBackupTargetLabels(backuptargetName, volumeName)).String(),
	})
}

// RemoveFinalizerForBackup will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackup(backup *longhorn.Backup) error {
	if !util.FinalizerExists(longhornFinalizerKey, backup) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, backup); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Backups(s.namespace).Update(context.TODO(), backup, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if backup.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backup %s", backup.Name)
	}
	return nil
}

// CreateSnapshot creates a Longhorn snapshot CR and verifies creation
func (s *DataStore) CreateSnapshot(snapshot *longhorn.Snapshot) (*longhorn.Snapshot, error) {
	ret, err := s.lhClient.LonghornV1beta2().Snapshots(s.namespace).Create(context.TODO(), snapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(snapshot.Name, "snapshot", func(name string) (k8sruntime.Object, error) {
		return s.GetSnapshotRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Snapshot)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for Snapshot")
	}
	return ret.DeepCopy(), nil
}

// GetSnapshotRO returns the Snapshot with the given snapshot name in the cluster
func (s *DataStore) GetSnapshotRO(snapName string) (*longhorn.Snapshot, error) {
	return s.snapshotLister.Snapshots(s.namespace).Get(snapName)
}

// GetSnapshot returns a copy of Snapshot with the given snapshot name in the cluster
func (s *DataStore) GetSnapshot(name string) (*longhorn.Snapshot, error) {
	resultRO, err := s.GetSnapshotRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateSnapshotStatus updates the given Longhorn snapshot status verifies update
func (s *DataStore) UpdateSnapshotStatus(snap *longhorn.Snapshot) (*longhorn.Snapshot, error) {
	obj, err := s.lhClient.LonghornV1beta2().Snapshots(s.namespace).UpdateStatus(context.TODO(), snap, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(snap.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSnapshotRO(name)
	})
	return obj, nil
}

// RemoveFinalizerForSnapshot will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForSnapshot(snapshot *longhorn.Snapshot) error {
	if !util.FinalizerExists(longhornFinalizerKey, snapshot) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, snapshot); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Snapshots(s.namespace).Update(context.TODO(), snapshot, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if snapshot.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for snapshot %s", snapshot.Name)
	}
	return nil
}

func (s *DataStore) ListSnapshotsRO(selector labels.Selector) (map[string]*longhorn.Snapshot, error) {
	list, err := s.snapshotLister.Snapshots(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}
	snapshots := make(map[string]*longhorn.Snapshot)
	for _, snap := range list {
		snapshots[snap.Name] = snap
	}
	return snapshots, nil
}

func (s *DataStore) ListSnapshots() (map[string]*longhorn.Snapshot, error) {
	list, err := s.snapshotLister.Snapshots(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	itemMap := make(map[string]*longhorn.Snapshot)
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) ListVolumeSnapshotsRO(volumeName string) (map[string]*longhorn.Snapshot, error) {
	selector, err := getVolumeSelector(volumeName)
	if err != nil {
		return nil, err
	}
	return s.ListSnapshotsRO(selector)
}

// DeleteSnapshot won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteSnapshot(snapshotName string) error {
	return s.lhClient.LonghornV1beta2().Snapshots(s.namespace).Delete(context.TODO(), snapshotName, metav1.DeleteOptions{})
}

// CreateRecurringJob creates a Longhorn RecurringJob resource and verifies
// creation
func (s *DataStore) CreateRecurringJob(recurringJob *longhorn.RecurringJob) (*longhorn.RecurringJob, error) {
	ret, err := s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).Create(context.TODO(), recurringJob, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "recurring job", func(name string) (k8sruntime.Object, error) {
		return s.getRecurringJobRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.RecurringJob)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for recurring job")
	}

	return ret.DeepCopy(), nil
}

// ListRecurringJobs returns a map of RecurringJobPolicies indexed by name
func (s *DataStore) ListRecurringJobs() (map[string]*longhorn.RecurringJob, error) {
	itemMap := map[string]*longhorn.RecurringJob{}

	list, err := s.recurringJobLister.RecurringJobs(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListRecurringJobsRO returns a map of RecurringJobPolicies indexed by name
func (s *DataStore) ListRecurringJobsRO() (map[string]*longhorn.RecurringJob, error) {
	itemMap := map[string]*longhorn.RecurringJob{}

	list, err := s.recurringJobLister.RecurringJobs(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

func (s *DataStore) getRecurringJobRO(name string) (*longhorn.RecurringJob, error) {
	return s.recurringJobLister.RecurringJobs(s.namespace).Get(name)
}

// GetRecurringJob gets the RecurringJob for the given name and namespace.
// Returns a mutable RecurringJob object
func (s *DataStore) GetRecurringJob(name string) (*longhorn.RecurringJob, error) {
	result, err := s.GetRecurringJobRO(name)
	if err != nil {
		return nil, err
	}
	return result.DeepCopy(), nil
}

func (s *DataStore) GetRecurringJobRO(name string) (*longhorn.RecurringJob, error) {
	return s.recurringJobLister.RecurringJobs(s.namespace).Get(name)
}

// UpdateRecurringJob updates Longhorn RecurringJob and verifies update
func (s *DataStore) UpdateRecurringJob(recurringJob *longhorn.RecurringJob) (*longhorn.RecurringJob, error) {
	obj, err := s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).Update(context.TODO(), recurringJob, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(recurringJob.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetRecurringJobRO(name)
	})
	return obj, nil
}

// UpdateRecurringJobStatus updates Longhorn RecurringJob resource status and
// verifies update
func (s *DataStore) UpdateRecurringJobStatus(recurringJob *longhorn.RecurringJob) (*longhorn.RecurringJob, error) {
	obj, err := s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).UpdateStatus(context.TODO(), recurringJob, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(recurringJob.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.getRecurringJobRO(name)
	})
	return obj, nil
}

// DeleteRecurringJob deletes the RecurringJob.
// The dependents will be deleted in the foreground
func (s *DataStore) DeleteRecurringJob(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
}

func ValidateRecurringJob(job longhorn.RecurringJobSpec) error {
	if job.Cron == "" || job.Task == "" || job.Name == "" {
		return fmt.Errorf("invalid job %+v", job)
	}
	if !isValidRecurringJobTask(job.Task) {
		return fmt.Errorf("recurring job task %v is not valid", job.Task)
	}
	if job.Concurrency == 0 {
		job.Concurrency = types.DefaultRecurringJobConcurrency
	}
	if _, err := cron.ParseStandard(job.Cron); err != nil {
		return fmt.Errorf("invalid cron format(%v): %v", job.Cron, err)
	}
	if len(job.Name) > NameMaximumLength {
		return fmt.Errorf("job name %v must be %v characters or less", job.Name, NameMaximumLength)
	}
	for _, group := range job.Groups {
		if !util.ValidateString(group) {
			return fmt.Errorf("invalid group name %v", group)
		}
	}
	if job.Labels != nil {
		if _, err := util.ValidateSnapshotLabels(job.Labels); err != nil {
			return err
		}
	}
	if job.Parameters != nil {
		if err := ValidateRecurringJobParameters(job.Task, job.Parameters); err != nil {
			return err
		}
	}
	return nil
}

func ValidateRecurringJobParameters(task longhorn.RecurringJobType, parameters map[string]string) (err error) {
	switch task {
	case longhorn.RecurringJobTypeBackup, longhorn.RecurringJobTypeBackupForceCreate:
		for key, value := range parameters {
			if err := validateRecurringJobBackupParameter(key, value); err != nil {
				return errors.Wrapf(err, "failed to validate recurring job backup task parameters")
			}
		}
	// we don't support any parameters for other tasks currently
	default:
		return nil
	}

	return nil
}

func validateRecurringJobBackupParameter(key, value string) error {
	switch key {
	case types.RecurringJobBackupParameterFullBackupInterval:
		_, err := strconv.Atoi(value)
		if err != nil {
			return errors.Wrapf(err, "%v:%v is not number", key, value)
		}
	default:
		return fmt.Errorf("%v:%v is not a valid parameter", key, value)
	}

	return nil
}

func isValidRecurringJobTask(task longhorn.RecurringJobType) bool {
	return task == longhorn.RecurringJobTypeBackup ||
		task == longhorn.RecurringJobTypeBackupForceCreate ||
		task == longhorn.RecurringJobTypeFilesystemTrim ||
		task == longhorn.RecurringJobTypeSnapshot ||
		task == longhorn.RecurringJobTypeSnapshotForceCreate ||
		task == longhorn.RecurringJobTypeSnapshotCleanup ||
		task == longhorn.RecurringJobTypeSnapshotDelete
}

// ValidateRecurringJobs validates data and formats for recurring jobs
func (s *DataStore) ValidateRecurringJobs(jobs []longhorn.RecurringJobSpec) error {
	if jobs == nil {
		return nil
	}

	totalJobRetainCount := 0
	for _, job := range jobs {
		if err := ValidateRecurringJob(job); err != nil {
			return err
		}
		totalJobRetainCount += job.Retain
	}

	maxRecurringJobRetain, err := s.GetSettingAsInt(types.SettingNameRecurringJobMaxRetention)
	if err != nil {
		return err
	}

	if totalJobRetainCount > int(maxRecurringJobRetain) {
		return fmt.Errorf("job Can't retain more than %d snapshots", maxRecurringJobRetain)
	}
	return nil
}

// CreateOrphan creates a Longhorn Orphan resource and verifies creation
func (s *DataStore) CreateOrphan(orphan *longhorn.Orphan) (*longhorn.Orphan, error) {
	ret, err := s.lhClient.LonghornV1beta2().Orphans(s.namespace).Create(context.TODO(), orphan, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "orphan", func(name string) (k8sruntime.Object, error) {
		return s.GetOrphanRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.Orphan)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for orphan")
	}

	return ret.DeepCopy(), nil
}

// GetOrphanRO returns the Orphan with the given orphan name in the cluster
func (s *DataStore) GetOrphanRO(orphanName string) (*longhorn.Orphan, error) {
	return s.orphanLister.Orphans(s.namespace).Get(orphanName)
}

// GetOrphan returns a copy of Orphan with the given orphan name in the cluster
func (s *DataStore) GetOrphan(name string) (*longhorn.Orphan, error) {
	resultRO, err := s.GetOrphanRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateOrphan updates the given Longhorn orphan in the cluster Orphan CR and verifies update
func (s *DataStore) UpdateOrphan(orphan *longhorn.Orphan) (*longhorn.Orphan, error) {
	obj, err := s.lhClient.LonghornV1beta2().Orphans(s.namespace).Update(context.TODO(), orphan, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(orphan.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetOrphanRO(name)
	})
	return obj, nil
}

// UpdateOrphanStatus updates the given Longhorn orphan status in the cluster Orphans CR status and verifies update
func (s *DataStore) UpdateOrphanStatus(orphan *longhorn.Orphan) (*longhorn.Orphan, error) {
	obj, err := s.lhClient.LonghornV1beta2().Orphans(s.namespace).UpdateStatus(context.TODO(), orphan, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(orphan.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetOrphanRO(name)
	})
	return obj, nil
}

// RemoveFinalizerForOrphan will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForOrphan(orphan *longhorn.Orphan) error {
	if !util.FinalizerExists(longhornFinalizerKey, orphan) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, orphan); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().Orphans(s.namespace).Update(context.TODO(), orphan, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if orphan.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for orphan %s", orphan.Name)
	}
	return nil
}

func (s *DataStore) listOrphans(selector labels.Selector) (map[string]*longhorn.Orphan, error) {
	list, err := s.orphanLister.Orphans(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.Orphan{}
	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListOrphans returns an object contains all Orphans for the given namespace
func (s *DataStore) ListOrphans() (map[string]*longhorn.Orphan, error) {
	return s.listOrphans(labels.Everything())
}

// ListOrphansByNode gets a map of Orphans on the node Name for the given namespace.
func (s *DataStore) ListOrphansByNode(name string) (map[string]*longhorn.Orphan, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.listOrphans(nodeSelector)
}

// ListOrphansRO returns a list of all Orphans for the given namespace
func (s *DataStore) ListOrphansRO() ([]*longhorn.Orphan, error) {
	return s.orphanLister.Orphans(s.namespace).List(labels.Everything())
}

// ListOrphansByNodeRO returns a list of all Orphans on node Name for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListOrphansByNodeRO(name string) ([]*longhorn.Orphan, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.orphanLister.Orphans(s.namespace).List(nodeSelector)
}

// DeleteOrphan won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteOrphan(orphanName string) error {
	return s.lhClient.LonghornV1beta2().Orphans(s.namespace).Delete(context.TODO(), orphanName, metav1.DeleteOptions{})
}

// GetOwnerReferencesForSupportBundle returns a list contains single OwnerReference for the
// given SupportBundle object
func GetOwnerReferencesForSupportBundle(supportBundle *longhorn.SupportBundle) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: longhorn.SchemeGroupVersion.String(),
			Kind:       types.LonghornKindSupportBundle,
			Name:       supportBundle.Name,
			UID:        supportBundle.UID,
		},
	}
}

// GetSupportBundleRO returns the SupportBundle with the given name
func (s *DataStore) GetSupportBundleRO(name string) (*longhorn.SupportBundle, error) {
	return s.supportBundleLister.SupportBundles(s.namespace).Get(name)
}

// GetSupportBundle returns a copy of SupportBundle with the given name
func (s *DataStore) GetSupportBundle(name string) (*longhorn.SupportBundle, error) {
	resultRO, err := s.GetSupportBundleRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// CreateLHVolumeAttachment creates a Longhorn volume attachment resource and verifies creation
func (s *DataStore) CreateLHVolumeAttachment(va *longhorn.VolumeAttachment) (*longhorn.VolumeAttachment, error) {
	ret, err := s.lhClient.LonghornV1beta2().VolumeAttachments(s.namespace).Create(context.TODO(), va, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "Longhorn VolumeAttachment", func(name string) (k8sruntime.Object, error) {
		return s.GetLHVolumeAttachmentRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.VolumeAttachment)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for Longhorn VolumeAttachment")
	}

	return ret.DeepCopy(), nil
}

// GetLHVolumeAttachmentRO returns the VolumeAttachment with the given name in the cluster
func (s *DataStore) GetLHVolumeAttachmentRO(name string) (*longhorn.VolumeAttachment, error) {
	return s.lhVolumeAttachmentLister.VolumeAttachments(s.namespace).Get(name)
}

// GetLHVolumeAttachment returns a copy of VolumeAttachment with the given name in the cluster
func (s *DataStore) GetLHVolumeAttachment(name string) (*longhorn.VolumeAttachment, error) {
	resultRO, err := s.GetLHVolumeAttachmentRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) GetLHVolumeAttachmentByVolumeName(volName string) (*longhorn.VolumeAttachment, error) {
	vaName := types.GetLHVolumeAttachmentNameFromVolumeName(volName)
	return s.GetLHVolumeAttachment(vaName)
}

// ListSupportBundles returns an object contains all SupportBundles
func (s *DataStore) ListSupportBundles() (map[string]*longhorn.SupportBundle, error) {
	itemMap := make(map[string]*longhorn.SupportBundle)

	list, err := s.ListSupportBundlesRO()
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListSupportBundlesRO returns a list of all SupportBundles for the given namespace.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListSupportBundlesRO() ([]*longhorn.SupportBundle, error) {
	return s.supportBundleLister.SupportBundles(s.namespace).List(labels.Everything())
}

// RemoveFinalizerForSupportBundle will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForSupportBundle(supportBundle *longhorn.SupportBundle) (err error) {
	if !util.FinalizerExists(longhornFinalizerKey, supportBundle) {
		// finalizer already removed
		return nil
	}

	if err = util.RemoveFinalizer(longhornFinalizerKey, supportBundle); err != nil {
		return err
	}

	supportBundle, err = s.lhClient.LonghornV1beta2().SupportBundles(s.namespace).Update(context.TODO(), supportBundle, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if supportBundle.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for SupportBundle %s", supportBundle.Name)
	}
	return nil
}

// UpdateSupportBundleStatus updates the given Longhorn SupportBundle status and verifies update
func (s *DataStore) UpdateSupportBundleStatus(supportBundle *longhorn.SupportBundle) (*longhorn.SupportBundle, error) {
	obj, err := s.lhClient.LonghornV1beta2().SupportBundles(s.namespace).UpdateStatus(context.TODO(), supportBundle, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(supportBundle.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSupportBundleRO(name)
	})
	return obj, nil
}

// CreateSupportBundle creates a Longhorn SupportBundle resource and verifies creation
func (s *DataStore) CreateSupportBundle(supportBundle *longhorn.SupportBundle) (*longhorn.SupportBundle, error) {
	ret, err := s.lhClient.LonghornV1beta2().SupportBundles(s.namespace).Create(context.TODO(), supportBundle, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "support bundle", func(name string) (k8sruntime.Object, error) {
		return s.GetSupportBundleRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.SupportBundle)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for support bundle")
	}

	return ret.DeepCopy(), nil
}

// DeleteSupportBundle won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteSupportBundle(name string) error {
	return s.lhClient.LonghornV1beta2().SupportBundles(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// CreateSystemBackup creates a Longhorn SystemBackup and verifies creation
func (s *DataStore) CreateSystemBackup(systemBackup *longhorn.SystemBackup) (*longhorn.SystemBackup, error) {
	ret, err := s.lhClient.LonghornV1beta2().SystemBackups(s.namespace).Create(context.TODO(), systemBackup, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "system backup", func(name string) (k8sruntime.Object, error) {
		return s.GetSystemBackupRO(name)
	})
	if err != nil {
		return nil, err
	}

	ret, ok := obj.(*longhorn.SystemBackup)
	if !ok {
		return nil, errors.Errorf("BUG: datastore: verifyCreation returned wrong type for SystemBackup")
	}
	return ret.DeepCopy(), nil
}

// DeleteSystemBackup won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteSystemBackup(name string) error {
	return s.lhClient.LonghornV1beta2().SystemBackups(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForSystemBackup results in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForSystemBackup(obj *longhorn.SystemBackup) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}

	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}

	_, err := s.lhClient.LonghornV1beta2().SystemBackups(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for SystemBackup %v", obj.Name)
	}
	return nil
}

// UpdateSystemBackup updates Longhorn SystemBackup and verifies update
func (s *DataStore) UpdateSystemBackup(systemBackup *longhorn.SystemBackup) (*longhorn.SystemBackup, error) {
	obj, err := s.lhClient.LonghornV1beta2().SystemBackups(s.namespace).Update(context.TODO(), systemBackup, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(systemBackup.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSystemBackupRO(name)
	})
	return obj, nil
}

// UpdateSystemBackupStatus updates Longhorn SystemBackup status and verifies update
func (s *DataStore) UpdateSystemBackupStatus(systemBackup *longhorn.SystemBackup) (*longhorn.SystemBackup, error) {
	obj, err := s.lhClient.LonghornV1beta2().SystemBackups(s.namespace).UpdateStatus(context.TODO(), systemBackup, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	verifyUpdate(systemBackup.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSystemBackupRO(name)
	})
	return obj, nil
}

// GetSystemBackup returns a copy of SystemBackup with the given name
func (s *DataStore) GetSystemBackup(name string) (*longhorn.SystemBackup, error) {
	resultRO, err := s.GetSystemBackupRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// GetSystemBackupRO returns the SystemBackup with the given name
func (s *DataStore) GetSystemBackupRO(name string) (*longhorn.SystemBackup, error) {
	return s.systemBackupLister.SystemBackups(s.namespace).Get(name)
}

// ListSystemBackups returns a copy of the object contains all SystemBackups
func (s *DataStore) ListSystemBackups() (map[string]*longhorn.SystemBackup, error) {
	list, err := s.ListSystemBackupsRO()
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.SystemBackup{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListSystemBackupsRO returns an object contains all SystemBackups
func (s *DataStore) ListSystemBackupsRO() ([]*longhorn.SystemBackup, error) {
	return s.systemBackupLister.SystemBackups(s.namespace).List(labels.Everything())
}

func LabelSystemBackupVersion(version string, obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetVersionLabelKey()] = version
	metadata.SetLabels(labels)
	return nil
}

// CreateSystemRestore creates a Longhorn SystemRestore resource and verifies creation
func (s *DataStore) CreateSystemRestore(systemRestore *longhorn.SystemRestore) (*longhorn.SystemRestore, error) {
	ret, err := s.lhClient.LonghornV1beta2().SystemRestores(s.namespace).Create(context.TODO(), systemRestore, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "system restore", func(name string) (k8sruntime.Object, error) {
		return s.GetSystemRestoreRO(name)
	})
	if err != nil {
		return nil, err
	}

	ret, ok := obj.(*longhorn.SystemRestore)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for SystemRestore")
	}

	return ret.DeepCopy(), nil
}

// DeleteSystemRestore won't result in immediately deletion since finalizer was set by default
// The dependents will be deleted in the foreground
func (s *DataStore) DeleteSystemRestore(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.lhClient.LonghornV1beta2().SystemRestores(s.namespace).Delete(
		context.TODO(),
		name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
}

// GetOwnerReferencesForSystemRestore returns OwnerReference for the given
// SystemRestore
func GetOwnerReferencesForSystemRestore(systemRestore *longhorn.SystemRestore) []metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{
		{
			APIVersion:         longhorn.SchemeGroupVersion.String(),
			Kind:               types.LonghornKindSystemRestore,
			Name:               systemRestore.Name,
			UID:                systemRestore.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		},
	}
}

// RemoveSystemRestoreLabel removed the system-restore label and updates the SystemRestore. Longhorn labels
// SystemRestore with "longhorn.io/system-restore: InProgress" during restoring to ensure only one restore is rolling out.
func (s *DataStore) RemoveSystemRestoreLabel(systemRestore *longhorn.SystemRestore) (*longhorn.SystemRestore, error) {
	key := types.GetSystemRestoreLabelKey()
	if _, exist := systemRestore.Labels[key]; !exist {
		return systemRestore, nil
	}

	delete(systemRestore.Labels, key)
	_, err := s.lhClient.LonghornV1beta2().SystemRestores(s.namespace).Update(context.TODO(), systemRestore, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to remove SystemRestore %v label %v", systemRestore.Name, key)
	}

	log := logrus.WithFields(logrus.Fields{
		"systemRestore": systemRestore.Name,
		"label":         key,
	})
	log.Info("Removed SystemRestore label")
	return systemRestore, nil
}

// UpdateSystemRestore updates Longhorn SystemRestore and verifies update
func (s *DataStore) UpdateSystemRestore(systemRestore *longhorn.SystemRestore) (*longhorn.SystemRestore, error) {
	err := labelLonghornSystemRestoreInProgress(systemRestore)
	if err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().SystemRestores(s.namespace).Update(context.TODO(), systemRestore, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	verifyUpdate(systemRestore.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSystemRestore(name)
	})

	return obj, nil
}

func labelLonghornSystemRestoreInProgress(obj k8sruntime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.GetSystemRestoreLabelKey()] = string(longhorn.SystemRestoreStateInProgress)
	metadata.SetLabels(labels)
	return nil
}

// UpdateSystemRestoreStatus updates Longhorn SystemRestore resource status and verifies update
func (s *DataStore) UpdateSystemRestoreStatus(systemRestore *longhorn.SystemRestore) (*longhorn.SystemRestore, error) {
	obj, err := s.lhClient.LonghornV1beta2().SystemRestores(s.namespace).UpdateStatus(context.TODO(), systemRestore, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	verifyUpdate(systemRestore.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetSystemRestoreRO(name)
	})

	return obj, nil
}

// GetSystemRestore returns a copy of SystemRestore with the given obj name
func (s *DataStore) GetSystemRestore(name string) (*longhorn.SystemRestore, error) {
	resultRO, err := s.GetSystemRestoreRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// GetSystemRestoreRO returns the SystemRestore with the given CR name
func (s *DataStore) GetSystemRestoreRO(name string) (*longhorn.SystemRestore, error) {
	return s.systemRestoreLister.SystemRestores(s.namespace).Get(name)
}

// GetSystemRestoreInProgress validate the given name and returns the only
// SystemRestore in progress. Returns error if found less or more SystemRestore.
// If given an empty name, return the SystemRestore without validating the name.
func (s *DataStore) GetSystemRestoreInProgress(name string) (systemRestore *longhorn.SystemRestore, err error) {
	systemRestores, err := s.ListSystemRestoresInProgress()
	if err != nil {
		return nil, err
	}

	systemRestoreCount := len(systemRestores)
	if systemRestoreCount == 0 {
		return nil, errors.Errorf("cannot find in-progress system restore")
	} else if systemRestoreCount > 1 {
		return nil, errors.Errorf("found %v system restore already in progress", systemRestoreCount-1)
	}

	for _, systemRestore = range systemRestores {
		if name == "" {
			break
		}

		if systemRestore.Name != name {
			return nil, errors.Errorf("found the in-progress system restore with a mismatching name %v, expecting %v", systemRestore.Name, name)
		}
	}

	return systemRestore, nil
}

// ListSystemRestoresInProgress returns an object contains all SystemRestores in progress
func (s *DataStore) ListSystemRestoresInProgress() (map[string]*longhorn.SystemRestore, error) {
	selector, err := s.getSystemRestoreInProgressSelector()
	if err != nil {
		return nil, err
	}
	return s.listSystemRestores(selector)
}

func (s *DataStore) getSystemRestoreInProgressSelector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetSystemRestoreInProgressLabel(),
	})
}

func (s *DataStore) listSystemRestores(selector labels.Selector) (map[string]*longhorn.SystemRestore, error) {
	list, err := s.systemRestoreLister.SystemRestores(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.SystemRestore{}
	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListSystemRestores returns an object contains all SystemRestores
func (s *DataStore) ListSystemRestores() (map[string]*longhorn.SystemRestore, error) {
	return s.listSystemRestores(labels.Everything())
}

// UpdateLHVolumeAttachment updates the given Longhorn VolumeAttachment in the VolumeAttachment CR and verifies update
func (s *DataStore) UpdateLHVolumeAttachment(va *longhorn.VolumeAttachment) (*longhorn.VolumeAttachment, error) {
	obj, err := s.lhClient.LonghornV1beta2().VolumeAttachments(s.namespace).Update(context.TODO(), va, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(va.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetLHVolumeAttachmentRO(name)
	})
	return obj, nil
}

// UpdateLHVolumeAttachmentStatus updates the given Longhorn VolumeAttachment status in the VolumeAttachment CR status and verifies update
func (s *DataStore) UpdateLHVolumeAttachmentStatus(va *longhorn.VolumeAttachment) (*longhorn.VolumeAttachment, error) {
	obj, err := s.lhClient.LonghornV1beta2().VolumeAttachments(s.namespace).UpdateStatus(context.TODO(), va, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(va.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetLHVolumeAttachmentRO(name)
	})
	return obj, nil
}

// ListLonghornVolumeAttachmentByVolumeRO returns a list of all Longhorn VolumeAttachments of a volume.
// The list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListLonghornVolumeAttachmentByVolumeRO(name string) ([]*longhorn.VolumeAttachment, error) {
	volumeSelector, err := getVolumeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.lhVolumeAttachmentLister.VolumeAttachments(s.namespace).List(volumeSelector)
}

// ListLHVolumeAttachmentsRO returns a list of all VolumeAttachments for the given namespace
func (s *DataStore) ListLHVolumeAttachmentsRO() ([]*longhorn.VolumeAttachment, error) {
	return s.lhVolumeAttachmentLister.VolumeAttachments(s.namespace).List(labels.Everything())
}

// RemoveFinalizerForLHVolumeAttachment will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForLHVolumeAttachment(va *longhorn.VolumeAttachment) error {
	if !util.FinalizerExists(longhornFinalizerKey, va) {
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, va); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().VolumeAttachments(s.namespace).Update(context.TODO(), va, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if va.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for VolumeAttachment %s", va.Name)
	}
	return nil
}

// DeleteLHVolumeAttachment won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteLHVolumeAttachment(vaName string) error {
	return s.lhClient.LonghornV1beta2().VolumeAttachments(s.namespace).Delete(context.TODO(), vaName, metav1.DeleteOptions{})
}

// IsSupportedVolumeSize returns turn if the v1 volume size is supported by the given fsType file system.
func IsSupportedVolumeSize(dataEngine longhorn.DataEngineType, fsType string, volumeSize int64) bool {
	// TODO: check the logical volume maximum size limit
	if types.IsDataEngineV1(dataEngine) {
		// unix.Statfs can not differentiate the ext2/ext3/ext4 file systems.
		if (strings.HasPrefix(fsType, "ext") && volumeSize >= util.MaxExt4VolumeSize) || (fsType == "xfs" && volumeSize >= util.MaxXfsVolumeSize) {
			return false
		}
	}
	return true
}

// IsDataEngineEnabled returns true if the given dataEngine is enabled
func (s *DataStore) IsDataEngineEnabled(dataEngine longhorn.DataEngineType) (bool, error) {
	dataEngineSetting := types.SettingNameV1DataEngine
	if types.IsDataEngineV2(dataEngine) {
		dataEngineSetting = types.SettingNameV2DataEngine
	}

	enabled, err := s.GetSettingAsBool(dataEngineSetting)
	if err != nil {
		return true, errors.Wrapf(err, "failed to get %v setting", dataEngineSetting)
	}

	return enabled, nil
}

func (s *DataStore) GetDataEngines() map[longhorn.DataEngineType]struct{} {
	dataEngines := map[longhorn.DataEngineType]struct{}{}

	for _, setting := range []types.SettingName{types.SettingNameV1DataEngine, types.SettingNameV2DataEngine} {
		dataEngineEnabled, err := s.GetSettingAsBool(setting)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get setting %v", setting)
			continue
		}
		if dataEngineEnabled {
			switch setting {
			case types.SettingNameV1DataEngine:
				dataEngines[longhorn.DataEngineTypeV1] = struct{}{}
			case types.SettingNameV2DataEngine:
				dataEngines[longhorn.DataEngineTypeV2] = struct{}{}
			}
		}
	}

	return dataEngines
}

// IsV2VolumeDisabledForNode returns true if the node disables v2 data engine
func (s *DataStore) IsV2DataEngineDisabledForNode(nodeName string) (bool, error) {
	kubeNode, err := s.GetKubernetesNodeRO(nodeName)
	if err != nil {
		return false, err
	}
	val, ok := kubeNode.Labels[types.NodeDisableV2DataEngineLabelKey]
	if ok && val == types.NodeDisableV2DataEngineLabelKeyTrue {
		return true, nil
	}
	return false, nil
}

func (s *DataStore) GetDiskBackingImageMap() (map[string][]*longhorn.BackingImage, error) {
	diskBackingImageMap := map[string][]*longhorn.BackingImage{}
	backingImages, err := s.ListBackingImages()
	if err != nil {
		return nil, err
	}

	for _, bi := range backingImages {
		for diskUUID := range bi.Status.DiskFileStatusMap {
			diskBackingImageMap[diskUUID] = append(diskBackingImageMap[diskUUID], bi)
		}
	}

	return diskBackingImageMap, nil
}

// CreateBackupBackingImage creates a Longhorn BackupBackingImage resource and verifies
func (s *DataStore) CreateBackupBackingImage(backupBackingImage *longhorn.BackupBackingImage) (*longhorn.BackupBackingImage, error) {
	ret, err := s.lhClient.LonghornV1beta2().BackupBackingImages(s.namespace).Create(context.TODO(), backupBackingImage, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backup backing image", func(name string) (k8sruntime.Object, error) {
		return s.GetBackupBackingImageRO(name)
	})
	if err != nil {
		return nil, err
	}
	ret, ok := obj.(*longhorn.BackupBackingImage)
	if !ok {
		return nil, fmt.Errorf("BUG: datastore: verifyCreation returned wrong type for backup backing image")
	}

	return ret.DeepCopy(), nil
}

// UpdateBackupBackingImage updates Longhorn BackupBackingImage and verifies update
func (s *DataStore) UpdateBackupBackingImage(backupBackingImage *longhorn.BackupBackingImage) (*longhorn.BackupBackingImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackupBackingImages(s.namespace).Update(context.TODO(), backupBackingImage, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupBackingImage.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupBackingImageRO(name)
	})
	return obj, nil
}

// UpdateBackupBackingImageStatus updates Longhorn BackupBackingImage resource status and
// verifies update
func (s *DataStore) UpdateBackupBackingImageStatus(backupBackingImage *longhorn.BackupBackingImage) (*longhorn.BackupBackingImage, error) {
	obj, err := s.lhClient.LonghornV1beta2().BackupBackingImages(s.namespace).UpdateStatus(context.TODO(), backupBackingImage, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupBackingImage.Name, obj, func(name string) (k8sruntime.Object, error) {
		return s.GetBackupBackingImageRO(name)
	})
	return obj, nil
}

// DeleteBackupBackingImage won't result in immediately deletion since finalizer was
// set by default
func (s *DataStore) DeleteBackupBackingImage(name string) error {
	return s.lhClient.LonghornV1beta2().BackupBackingImages(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// RemoveFinalizerForBackupBackingImage will result in deletion if DeletionTimestamp was set
func (s *DataStore) RemoveFinalizerForBackupBackingImage(obj *longhorn.BackupBackingImage) error {
	if !util.FinalizerExists(longhornFinalizerKey, obj) {
		// finalizer already removed
		return nil
	}
	if err := util.RemoveFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
	}
	_, err := s.lhClient.LonghornV1beta2().BackupBackingImages(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
	if err != nil {
		// workaround `StorageError: invalid object, Code: 4` due to empty object
		if obj.DeletionTimestamp != nil {
			return nil
		}
		return errors.Wrapf(err, "unable to remove finalizer for backing image %v", obj.Name)
	}
	return nil
}

func (s *DataStore) GetBackupBackingImageRO(name string) (*longhorn.BackupBackingImage, error) {
	return s.backupBackingImageLister.BackupBackingImages(s.namespace).Get(name)
}

// GetBackupBackingImage returns a new BackupBackingImage object for the given name and
// namespace
func (s *DataStore) GetBackupBackingImage(name string) (*longhorn.BackupBackingImage, error) {
	resultRO, err := s.GetBackupBackingImageRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// GetBackupBackingImagesWithBackupTargetBackingImageRO returns a new BackupBackingImage object for the given backup target and backing image name
func (s *DataStore) GetBackupBackingImagesWithBackupTargetNameRO(backupTargetName, backingImageName string) (*longhorn.BackupBackingImage, error) {
	selector, err := getBackingImageWithBackupTargetSelector(backupTargetName, backingImageName)
	if err != nil {
		return nil, err
	}

	list, err := s.backupBackingImageLister.BackupBackingImages(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	if len(list) >= 2 {
		return nil, fmt.Errorf("datastore: found more than one backup backing image with backup target %v and backing image %v", backupTargetName, backingImageName)
	}

	for _, itemRO := range list {
		return itemRO, nil
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "backupBackingImages"}, "")
}

func getBackingImageWithBackupTargetSelector(backupTargetName, backingImageName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackingImageWithBackupTargetLabels(backupTargetName, backingImageName),
	})
}

// ListBackupBackingImages returns object includes all BackupBackingImage in namespace
func (s *DataStore) ListBackupBackingImages() (map[string]*longhorn.BackupBackingImage, error) {
	itemMap := map[string]*longhorn.BackupBackingImage{}

	list, err := s.backupBackingImageLister.BackupBackingImages(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListBackupBackingImagesWithBackupTargetNameRO returns object includes all BackupBackingImage in namespace
func (s *DataStore) ListBackupBackingImagesWithBackupTargetNameRO(backupTargetName string) (map[string]*longhorn.BackupBackingImage, error) {
	itemMap := map[string]*longhorn.BackupBackingImage{}

	selector, err := getBackupTargetSelector(backupTargetName)
	if err != nil {
		return nil, err
	}

	list, err := s.backupBackingImageLister.BackupBackingImages(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO
	}
	return itemMap, nil
}

func (s *DataStore) ListBackupBackingImagesRO() ([]*longhorn.BackupBackingImage, error) {
	return s.backupBackingImageLister.BackupBackingImages(s.namespace).List(labels.Everything())
}

// GetRunningInstanceManagerByNodeRO returns the running instance manager for the given node and data engine
func (s *DataStore) GetRunningInstanceManagerByNodeRO(node string, dataEngine longhorn.DataEngineType) (*longhorn.InstanceManager, error) {
	// Trying to get the default instance manager first.
	// If the default instance manager is not running, then try to get another running instance manager.
	im, err := s.GetDefaultInstanceManagerByNodeRO(node, dataEngine)
	if err == nil {
		if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
			return im, nil
		}
	}

	logrus.WithError(err).Debugf("Failed to get the default instance manager for node %v and data engine %v, trying to get another running instance manager", node, dataEngine)

	ims, err := s.ListInstanceManagersByNodeRO(node, longhorn.InstanceManagerTypeAllInOne, dataEngine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list instance managers for node %v", node)
	}

	for _, im := range ims {
		if im.Status.CurrentState == longhorn.InstanceManagerStateRunning {
			return im, nil
		}
	}

	return nil, fmt.Errorf("failed to find a running instance manager for node %v", node)
}

func (s *DataStore) GetFreezeFilesystemForSnapshotSetting(e *longhorn.Engine) (bool, error) {
	volume, err := s.GetVolumeRO(e.Spec.VolumeName)
	if err != nil {
		return false, err
	}

	if volume.Spec.FreezeFilesystemForSnapshot != longhorn.FreezeFilesystemForSnapshotDefault {
		return volume.Spec.FreezeFilesystemForSnapshot == longhorn.FreezeFilesystemForSnapshotEnabled, nil
	}

	return s.GetSettingAsBool(types.SettingNameFreezeFilesystemForSnapshot)
}

func (s *DataStore) CanPutBackingImageOnDisk(backingImage *longhorn.BackingImage, diskUUID string) (bool, error) {
	node, diskName, err := s.GetReadyDiskNodeRO(diskUUID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get the node of the disk %v", diskUUID)
	}

	allowEmptyNodeSelectorVolume, err := s.GetSettingAsBool(types.SettingNameAllowEmptyNodeSelectorVolume)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyNodeSelectorVolume)
	}

	allowEmptyDiskSelectorVolume, err := s.GetSettingAsBool(types.SettingNameAllowEmptyDiskSelectorVolume)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get %v setting", types.SettingNameAllowEmptyDiskSelectorVolume)
	}

	if !types.IsSelectorsInTags(node.Spec.Tags, backingImage.Spec.NodeSelector, allowEmptyNodeSelectorVolume) {
		return false, nil
	}

	diskSpec, exists := node.Spec.Disks[diskName]
	if exists {
		if !types.IsSelectorsInTags(diskSpec.Tags, backingImage.Spec.DiskSelector, allowEmptyDiskSelectorVolume) {
			return false, nil
		}
	}
	return true, nil
}

func (s *DataStore) GetOneBackingImageReadyNodeDisk(backingImage *longhorn.BackingImage) (*longhorn.Node, string, error) {
	for diskUUID := range backingImage.Spec.DiskFileSpecMap {
		bimMap, err := s.ListBackingImageManagersByDiskUUID(diskUUID)
		if err != nil {
			return nil, "", errors.Wrapf(err, "failed to get backing image manager by disk uuid %v", diskUUID)
		}
		for _, bim := range bimMap {
			if bim.DeletionTimestamp == nil {
				if info, exists := bim.Status.BackingImageFileMap[backingImage.Name]; exists && info.State == longhorn.BackingImageStateReady {
					return s.GetReadyDiskNode(bim.Spec.DiskUUID)
				}
			}
		}
	}

	return nil, "", fmt.Errorf("failed to find one ready backing image %v", backingImage.Name)
}

func (s *DataStore) IsPVMountOptionReadOnly(volume *longhorn.Volume) (bool, error) {
	kubeStatus := volume.Status.KubernetesStatus
	if kubeStatus.PVName == "" {
		return false, nil
	}

	pv, err := s.GetPersistentVolumeRO(kubeStatus.PVName)
	if err != nil {
		return false, err
	}
	if pv != nil {
		for _, opt := range pv.Spec.MountOptions {
			if opt == "ro" {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *DataStore) IsStorageNetworkForRWXVolume() (bool, error) {
	storageNetworkSetting, err := s.GetSettingWithAutoFillingRO(types.SettingNameStorageNetwork)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to get setting %v", types.SettingNameStorageNetwork)
	}

	storageNetworkForRWXVolumeEnabled, err := s.GetSettingAsBool(types.SettingNameStorageNetworkForRWXVolumeEnabled)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to get setting %v", types.SettingNameStorageNetworkForRWXVolumeEnabled)
	}

	return types.IsStorageNetworkForRWXVolume(storageNetworkSetting, storageNetworkForRWXVolumeEnabled), nil
}

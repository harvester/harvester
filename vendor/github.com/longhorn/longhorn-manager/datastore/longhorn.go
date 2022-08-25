package datastore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	// NameMaximumLength restricted the length due to Kubernetes name limitation
	NameMaximumLength = 40

	MaxRecurringJobRetain = 50
)

var (
	longhornFinalizerKey = longhorn.SchemeGroupVersion.Group

	// VerificationRetryInterval is the wait time for each verification retries
	VerificationRetryInterval = 100 * time.Millisecond
	// VerificationRetryCounts is the number of times to retry for verification
	VerificationRetryCounts = 20
)

func (s *DataStore) UpdateCustomizedSettings(defaultImages map[types.SettingName]string) error {
	defaultSettingCM, err := s.GetConfigMap(s.namespace, types.DefaultDefaultSettingConfigMapName)
	if err != nil {
		return err
	}

	customizedDefaultSettings, err := types.GetCustomizedDefaultSettings(defaultSettingCM)
	if err != nil {
		return err
	}

	availableCustomizedDefaultSettings := s.filterCustomizedDefaultSettings(customizedDefaultSettings)

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
		_, err := s.GetSettingExact(sName)
		if err != nil && apierrors.IsNotFound(err) {
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

func (s *DataStore) filterCustomizedDefaultSettings(customizedDefaultSettings map[string]string) map[string]string {
	availableCustomizedDefaultSettings := make(map[string]string)

	for name, value := range customizedDefaultSettings {
		if err := s.ValidateSetting(string(name), value); err == nil {
			availableCustomizedDefaultSettings[name] = value
		} else {
			logrus.WithError(err).Errorf("invalid customized default setting %v with value %v, will continue applying other customized settings", name, value)
		}
	}
	return availableCustomizedDefaultSettings
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
		configMapResourceVersion := ""
		if s, err := s.GetSettingExact(sName); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else {
			if s.Annotations != nil {
				configMapResourceVersion = s.Annotations[types.GetLonghornLabelKey(types.ConfigMapResourceVersionKey)]
			}
		}

		definition, ok := types.GetSettingDefinition(sName)
		if !ok {
			return fmt.Errorf("BUG: setting %v is not defined", sName)
		}

		value, exist := customizedDefaultSettings[string(sName)]
		if !exist {
			continue
		}

		if configMapResourceVersion != defaultSettingCMResourceVersion {
			if definition.Required && value == "" {
				continue
			}
			if err := s.createOrUpdateSetting(sName, value, defaultSettingCMResourceVersion); err != nil {
				return err
			}
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
	obj, err := s.lhClient.LonghornV1beta2().Settings(s.namespace).Update(context.TODO(), setting, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(setting.Name, obj, func(name string) (runtime.Object, error) {
		return s.getSettingRO(name)
	})
	return obj, nil
}

// ValidateSetting checks the given setting value types and condition
func (s *DataStore) ValidateSetting(name, value string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to set the setting %v with invalid value %v", name, value)
	}()
	sName := types.SettingName(name)

	if err := types.ValidateSetting(name, value); err != nil {
		return err
	}

	switch sName {
	case types.SettingNameBackupTarget:
		vs, err := s.ListDRVolumesRO()
		if err != nil {
			return errors.Wrapf(err, "failed to list standby volume when modifying BackupTarget")
		}
		if len(vs) != 0 {
			standbyVolumeNames := make([]string, len(vs))
			for k := range vs {
				standbyVolumeNames = append(standbyVolumeNames, k)
			}
			return fmt.Errorf("cannot modify BackupTarget since there are existing standby volumes: %v", standbyVolumeNames)
		}
	case types.SettingNameBackupTargetCredentialSecret:
		secret, err := s.GetSecretRO(s.namespace, value)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get the secret before modifying backup target credential secret setting")
			}
			return nil
		}
		checkKeyList := []string{
			types.AWSAccessKey,
			types.AWSIAMRoleAnnotation,
			types.AWSIAMRoleArn,
			types.AWSAccessKey,
			types.AWSSecretKey,
			types.AWSEndPoint,
			types.AWSCert,
			types.HTTPSProxy,
			types.HTTPProxy,
			types.NOProxy,
			types.VirtualHostedStyle,
		}
		for _, checkKey := range checkKeyList {
			if value, ok := secret.Data[checkKey]; ok {
				if strings.TrimSpace(string(value)) != string(value) {
					return fmt.Errorf("there is space or new line in %s", checkKey)
				}
			}
		}
	case types.SettingNameTaintToleration:
		list, err := s.ListVolumesRO()
		if err != nil {
			return errors.Wrapf(err, "failed to list volumes before modifying toleration setting")
		}
		for _, v := range list {
			if v.Status.State != longhorn.VolumeStateDetached {
				return fmt.Errorf("cannot modify toleration setting before all volumes are detached")
			}
		}
	case types.SettingNameSystemManagedComponentsNodeSelector:
		list, err := s.ListVolumesRO()
		if err != nil {
			return errors.Wrapf(err, "failed to list volumes before modifying node selector for managed components setting")
		}
		for _, v := range list {
			if v.Status.State != longhorn.VolumeStateDetached {
				return fmt.Errorf("cannot modify node selector for managed components setting before all volumes are detached")
			}
		}
	case types.SettingNamePriorityClass:
		if value != "" {
			if _, err := s.GetPriorityClass(value); err != nil {
				return errors.Wrapf(err, "failed to get priority class %v before modifying priority class setting", value)
			}
		}
		list, err := s.ListVolumesRO()
		if err != nil {
			return errors.Wrapf(err, "failed to list volumes before modifying priority class setting")
		}
		for _, v := range list {
			if v.Status.State != longhorn.VolumeStateDetached {
				return fmt.Errorf("cannot modify priority class setting before all volumes are detached")
			}
		}
	case types.SettingNameGuaranteedEngineManagerCPU:
		fallthrough
	case types.SettingNameGuaranteedReplicaManagerCPU:
		guaranteedEngineManagerCPU, err := s.GetSetting(types.SettingNameGuaranteedEngineManagerCPU)
		if err != nil {
			return err
		}
		if sName == types.SettingNameGuaranteedEngineManagerCPU {
			guaranteedEngineManagerCPU.Value = value
		}
		guaranteedReplicaManagerCPU, err := s.GetSetting(types.SettingNameGuaranteedReplicaManagerCPU)
		if err != nil {
			return err
		}
		if sName == types.SettingNameGuaranteedReplicaManagerCPU {
			guaranteedReplicaManagerCPU.Value = value
		}
		if err := types.ValidateCPUReservationValues(guaranteedEngineManagerCPU.Value, guaranteedReplicaManagerCPU.Value); err != nil {
			return err
		}
	case types.SettingNameStorageNetwork:
		volumesDetached, err := s.AreAllVolumesDetached()
		if err != nil {
			return errors.Wrapf(err, "failed to check volume detachment for %v setting update", types.SettingNameStorageNetwork)
		}

		if !volumesDetached {
			return errors.Errorf("cannot apply %v setting to Longhorn workloads when there are attached volumes", types.SettingNameStorageNetwork)
		}
	}
	return nil
}

func (s *DataStore) AreAllVolumesDetached() (bool, error) {
	image, err := s.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
	if err != nil {
		return false, err
	}

	nodes, err := s.ListNodes()
	if err != nil {
		return false, err
	}

	for node := range nodes {
		engineIMs, err := s.ListInstanceManagersBySelector(node, image, longhorn.InstanceManagerTypeEngine)
		if err != nil {
			if ErrorIsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		for _, engineIM := range engineIMs {
			if len(engineIM.Status.Instances) > 0 {
				return false, nil
			}
		}
	}

	return true, nil
}

func (s *DataStore) getSettingRO(name string) (*longhorn.Setting, error) {
	return s.sLister.Settings(s.namespace).Get(name)
}

// GetSettingExact returns the Setting for the given name and namespace
func (s *DataStore) GetSettingExact(sName types.SettingName) (*longhorn.Setting, error) {
	resultRO, err := s.getSettingRO(string(sName))
	if err != nil {
		return nil, err
	}

	return resultRO.DeepCopy(), nil
}

// GetSetting will automatically fill the non-existing setting if it's a valid
// setting name.
// The function will not return nil for *longhorn.Setting when error is nil
func (s *DataStore) GetSetting(sName types.SettingName) (*longhorn.Setting, error) {
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
	return resultRO.DeepCopy(), nil
}

// GetSettingValueExisted returns the value of the given setting name.
// Returns error if the setting does not exist or value is empty
func (s *DataStore) GetSettingValueExisted(sName types.SettingName) (string, error) {
	setting, err := s.GetSetting(sName)
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

	list, err := s.sLister.Settings(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		settingField := types.SettingName(itemRO.Name)
		// Ignore the items that we don't recongize
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

func tagVolumeLabel(volumeName string, obj runtime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	for k, v := range types.GetVolumeLabels(volumeName) {
		labels[k] = v
	}
	metadata.SetLabels(labels)
	return nil
}

func fixupMetadata(volumeName string, obj runtime.Object) error {
	if err := tagVolumeLabel(volumeName, obj); err != nil {
		return err
	}
	if err := util.AddFinalizer(longhornFinalizerKey, obj); err != nil {
		return err
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

// CreateVolume creates a Longhorn Volume resource and verifies creation
func (s *DataStore) CreateVolume(v *longhorn.Volume) (*longhorn.Volume, error) {
	if err := fixupMetadata(v.Name, v); err != nil {
		return nil, err
	}
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

	obj, err := verifyCreation(ret.Name, "volume", func(name string) (runtime.Object, error) {
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
	if err := fixupMetadata(v.Name, v); err != nil {
		return nil, err
	}
	if err := FixupRecurringJob(v); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Volumes(s.namespace).Update(context.TODO(), v, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(v.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(v.Name, obj, func(name string) (runtime.Object, error) {
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
	return s.vLister.Volumes(s.namespace).Get(name)
}

// GetVolume returns a new volume object for the given namespace and name
func (s *DataStore) GetVolume(name string) (*longhorn.Volume, error) {
	resultRO, err := s.vLister.Volumes(s.namespace).Get(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListVolumesRO returns a list of all Volumes for the given namespace
func (s *DataStore) ListVolumesRO() ([]*longhorn.Volume, error) {
	return s.vLister.Volumes(s.namespace).List(labels.Everything())
}

// ListVolumesROWithBackupVolumeName returns a single object contains all volumes
// with the given backup volume name
func (s *DataStore) ListVolumesROWithBackupVolumeName(backupVolumeName string) ([]*longhorn.Volume, error) {
	selector, err := getBackupVolumeSelector(backupVolumeName)
	if err != nil {
		return nil, err
	}
	return s.vLister.Volumes(s.namespace).List(selector)
}

// ListVolumesBySelectorRO returns a list of all Volumes for the given namespace
func (s *DataStore) ListVolumesBySelectorRO(selector labels.Selector) ([]*longhorn.Volume, error) {
	return s.vLister.Volumes(s.namespace).List(selector)
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

func (s *DataStore) DeleteVolumeRecurringJob(name string, isGroup bool, v *longhorn.Volume) (volume *longhorn.Volume, err error) {
	vName := v.Name
	defer func() {
		err = errors.Wrapf(err, "failed to delete volume recurring jobs for %v", vName)
	}()

	key := ""
	if isGroup {
		key = types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJobGroup, name)
	} else {
		key = types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJob, name)
	}
	if _, exist := v.Labels[key]; exist {
		delete(v.Labels, key)
		v, err = s.UpdateVolume(v)
		if err != nil {
			return nil, err
		}
		logrus.Debugf("Updated volume %v labels to %+v", v.Name, v.Labels)
	}
	return v, nil
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

// ListDRVolumesROWithBackupVolumeName returns a single object contains the DR volumes
// matches to the backup volume name
func (s *DataStore) ListDRVolumesROWithBackupVolumeName(backupVolumeName string) (map[string]*longhorn.Volume, error) {
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
		return GetNewCurrentEngineAndExtras(v, es)
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
		if (v.Spec.NodeID != "" && v.Spec.NodeID == e.Spec.NodeID) ||
			(v.Status.CurrentNodeID != "" && v.Status.CurrentNodeID == e.Spec.NodeID) ||
			(v.Status.PendingNodeID != "" && v.Status.PendingNodeID == e.Spec.NodeID) {
			if currentEngine != nil {
				return nil, nil, fmt.Errorf("BUG: found the second new active engine %v besides %v", e.Name, currentEngine.Name)
			}
			currentEngine = e
			currentEngine.Spec.Active = true
		} else {
			extras = append(extras, e)
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
	es, err := s.ListVolumeEngines(volumeName)
	if err != nil {
		return nil, err
	}
	v, err := s.GetVolumeRO(volumeName)
	if err != nil {
		return nil, err
	}
	return s.PickVolumeCurrentEngine(v, es)
}

// CreateEngine creates a Longhorn Engine resource and verifies creation
func (s *DataStore) CreateEngine(e *longhorn.Engine) (*longhorn.Engine, error) {
	if err := checkEngine(e); err != nil {
		return nil, err
	}
	if err := fixupMetadata(e.Spec.VolumeName, e); err != nil {
		return nil, err
	}
	if err := tagNodeLabel(e.Spec.NodeID, e); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).Create(context.TODO(), e, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(e.Name, "engine", func(name string) (runtime.Object, error) {
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
	if err := fixupMetadata(e.Spec.VolumeName, e); err != nil {
		return nil, err
	}
	if err := tagNodeLabel(e.Spec.NodeID, e); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).Update(context.TODO(), e, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(e.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(e.Name, obj, func(name string) (runtime.Object, error) {
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
	return s.eLister.Engines(s.namespace).Get(name)
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
	list, err := s.eLister.Engines(s.namespace).List(selector)
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
	return s.eLister.Engines(s.namespace).List(labels.Everything())
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
	if err := fixupMetadata(r.Spec.VolumeName, r); err != nil {
		return nil, err
	}
	if err := tagNodeLabel(r.Spec.NodeID, r); err != nil {
		return nil, err
	}
	if err := tagDiskUUIDLabel(r.Spec.DiskID, r); err != nil {
		return nil, err
	}
	if err := tagBackingImageLabel(r.Spec.BackingImage, r); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).Create(context.TODO(), r, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "replica", func(name string) (runtime.Object, error) {
		return s.getReplicaRO(name)
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
	if err := fixupMetadata(r.Spec.VolumeName, r); err != nil {
		return nil, err
	}
	if err := tagNodeLabel(r.Spec.NodeID, r); err != nil {
		return nil, err
	}
	if err := tagDiskUUIDLabel(r.Spec.DiskID, r); err != nil {
		return nil, err
	}
	if err := tagBackingImageLabel(r.Spec.BackingImage, r); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().Replicas(s.namespace).Update(context.TODO(), r, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(r.Name, obj, func(name string) (runtime.Object, error) {
		return s.getReplicaRO(name)
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
	verifyUpdate(r.Name, obj, func(name string) (runtime.Object, error) {
		return s.getReplicaRO(name)
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
	resultRO, err := s.getReplicaRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) getReplicaRO(name string) (*longhorn.Replica, error) {
	return s.rLister.Replicas(s.namespace).Get(name)
}

func (s *DataStore) listReplicas(selector labels.Selector) (map[string]*longhorn.Replica, error) {
	list, err := s.rLister.Replicas(s.namespace).List(selector)
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
	return s.rLister.Replicas(s.namespace).List(labels.Everything())
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

	obj, err := verifyCreation(ret.Name, "engine image", func(name string) (runtime.Object, error) {
		return s.getEngineImageRO(name)
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
	if err := util.AddFinalizer(longhornFinalizerKey, img); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Update(context.TODO(), img, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(img.Name, obj, func(name string) (runtime.Object, error) {
		return s.getEngineImageRO(name)
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
	verifyUpdate(img.Name, obj, func(name string) (runtime.Object, error) {
		return s.getEngineImageRO(name)
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

func (s *DataStore) getEngineImageRO(name string) (*longhorn.EngineImage, error) {
	return s.iLister.EngineImages(s.namespace).Get(name)
}

// GetEngineImage returns a new EngineImage object for the given name and
// namespace
func (s *DataStore) GetEngineImage(name string) (*longhorn.EngineImage, error) {
	resultRO, err := s.getEngineImageRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListEngineImages returns object includes all EngineImage in namespace
func (s *DataStore) ListEngineImages() (map[string]*longhorn.EngineImage, error) {
	itemMap := map[string]*longhorn.EngineImage{}

	list, err := s.iLister.EngineImages(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// CheckEngineImageReadiness return true if the engine IMAGE is deployed on all nodes in the NODES list
func (s *DataStore) CheckEngineImageReadiness(image string, nodes ...string) (isReady bool, err error) {
	if len(nodes) == 0 || (len(nodes) == 1 && nodes[0] == "") {
		return false, nil
	}
	ei, err := s.GetEngineImage(types.GetEngineImageChecksumName(image))
	if err != nil {
		return false, fmt.Errorf("unable to get engine image %v: %v", image, err)
	}
	if ei.Status.State != longhorn.EngineImageStateDeployed && ei.Status.State != longhorn.EngineImageStateDeploying {
		return false, nil
	}
	nodesHaveEngineImage, err := s.ListNodesWithEngineImage(ei)
	if err != nil {
		return false, errors.Wrapf(err, "failed CheckEngineImageReadiness for nodes %v", nodes)
	}
	undeployedNodes := []string{}
	for _, node := range nodes {
		if _, ok := nodesHaveEngineImage[node]; !ok {
			undeployedNodes = append(undeployedNodes, node)
		}
	}
	if len(undeployedNodes) > 0 {
		logrus.Debugf("CheckEngineImageReadiness: nodes %v don't have the engine image %v", undeployedNodes, image)
		return false, nil
	}
	return true, nil
}

// CheckEngineImageReadyOnAtLeastOneVolumeReplica checks if the IMAGE is deployed on the NODEID and on at least one of the the volume's replicas
func (s *DataStore) CheckEngineImageReadyOnAtLeastOneVolumeReplica(image, volumeName, nodeID string) (bool, error) {
	replicas, err := s.ListVolumeReplicas(volumeName)
	if err != nil {
		return false, fmt.Errorf("cannot get replicas for volume %v: %w", volumeName, err)
	}

	isReady, err := s.CheckEngineImageReadiness(image, nodeID)
	if nodeID != "" && !isReady {
		return isReady, err
	}
	for _, r := range replicas {
		isReady, err := s.CheckEngineImageReadiness(image, r.Spec.NodeID)
		if err != nil || isReady {
			return isReady, err
		}
	}
	return false, nil
}

// CheckEngineImageReadyOnAllVolumeReplicas checks if the IMAGE is deployed on the NODEID as well as all the volume's replicas
func (s *DataStore) CheckEngineImageReadyOnAllVolumeReplicas(image, volumeName, nodeID string) (bool, error) {
	replicas, err := s.ListVolumeReplicas(volumeName)
	if err != nil {
		return false, fmt.Errorf("cannot get replicas for volume %v: %w", volumeName, err)
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
	return s.CheckEngineImageReadiness(image, nodes...)
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

	obj, err := verifyCreation(ret.Name, "backing image", func(name string) (runtime.Object, error) {
		return s.getBackingImageRO(name)
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
	if err := util.AddFinalizer(longhornFinalizerKey, backingImage); err != nil {
		return nil, err
	}
	obj, err := s.lhClient.LonghornV1beta2().BackingImages(s.namespace).Update(context.TODO(), backingImage, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImage.Name, obj, func(name string) (runtime.Object, error) {
		return s.getBackingImageRO(name)
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
	verifyUpdate(backingImage.Name, obj, func(name string) (runtime.Object, error) {
		return s.getBackingImageRO(name)
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

func (s *DataStore) getBackingImageRO(name string) (*longhorn.BackingImage, error) {
	return s.biLister.BackingImages(s.namespace).Get(name)
}

// GetBackingImage returns a new BackingImage object for the given name and
// namespace
func (s *DataStore) GetBackingImage(name string) (*longhorn.BackingImage, error) {
	resultRO, err := s.getBackingImageRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListBackingImages returns object includes all BackingImage in namespace
func (s *DataStore) ListBackingImages() (map[string]*longhorn.BackingImage, error) {
	itemMap := map[string]*longhorn.BackingImage{}

	list, err := s.biLister.BackingImages(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
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
	if err := tagLonghornNodeLabel(backingImageManager.Spec.NodeID, backingImageManager); err != nil {
		return nil, err
	}
	if err := tagLonghornDiskUUIDLabel(backingImageManager.Spec.DiskUUID, backingImageManager); err != nil {
		return nil, err
	}

	ret, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Create(context.TODO(), backingImageManager, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backing image manager", func(name string) (runtime.Object, error) {
		return s.getBackingImageManagerRO(name)
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
	if err := util.AddFinalizer(longhornFinalizerKey, backingImageManager); err != nil {
		return nil, err
	}
	if err := tagLonghornNodeLabel(backingImageManager.Spec.NodeID, backingImageManager); err != nil {
		return nil, err
	}
	if err := tagLonghornDiskUUIDLabel(backingImageManager.Spec.DiskUUID, backingImageManager); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().BackingImageManagers(s.namespace).Update(context.TODO(), backingImageManager, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageManager.Name, obj, func(name string) (runtime.Object, error) {
		return s.getBackingImageManagerRO(name)
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
	verifyUpdate(backingImageManager.Name, obj, func(name string) (runtime.Object, error) {
		return s.getBackingImageManagerRO(name)
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

func (s *DataStore) getBackingImageManagerRO(name string) (*longhorn.BackingImageManager, error) {
	return s.bimLister.BackingImageManagers(s.namespace).Get(name)
}

// GetBackingImageManager returns a new BackingImageManager object for the given name and
// namespace
func (s *DataStore) GetBackingImageManager(name string) (*longhorn.BackingImageManager, error) {
	resultRO, err := s.getBackingImageManagerRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) listBackingImageManagers(selector labels.Selector) (map[string]*longhorn.BackingImageManager, error) {
	itemMap := map[string]*longhorn.BackingImageManager{}

	list, err := s.bimLister.BackingImageManagers(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// ListBackingImageManagers returns object includes all BackingImageManager in namespace
func (s *DataStore) ListBackingImageManagers() (map[string]*longhorn.BackingImageManager, error) {
	return s.listBackingImageManagers(labels.Everything())
}

// ListBackingImageManagersByNode gets a list of BackingImageManager
// on a specific node with the given namespace.
func (s *DataStore) ListBackingImageManagersByNode(nodeName string) (map[string]*longhorn.BackingImageManager, error) {
	nodeSelector, err := getNodeSelector(nodeName)
	if err != nil {
		return nil, err
	}
	return s.listBackingImageManagers(nodeSelector)
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

// ListDefaultBackingImageManagers gets a list of BackingImageManager
// using default image with the given namespace.
func (s *DataStore) ListDefaultBackingImageManagers() (map[string]*longhorn.BackingImageManager, error) {
	defaultImage, err := s.GetSettingValueExisted(types.SettingNameDefaultBackingImageManagerImage)
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackingImageManager{}
	list, err := s.bimLister.BackingImageManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, itemRO := range list {
		if itemRO.Spec.Image != defaultImage {
			continue
		}
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
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
	if err := util.AddFinalizer(longhornFinalizerKey, backingImageDataSource); err != nil {
		return nil, err
	}
	ret, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Create(context.TODO(), backingImageDataSource, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backing image data source", func(name string) (runtime.Object, error) {
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
	if err := util.AddFinalizer(longhornFinalizerKey, backingImageDataSource); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().BackingImageDataSources(s.namespace).Update(context.TODO(), backingImageDataSource, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backingImageDataSource.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(backingImageDataSource.Name, obj, func(name string) (runtime.Object, error) {
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
	return s.bidsLister.BackingImageDataSources(s.namespace).Get(name)
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

	list, err := s.bidsLister.BackingImageDataSources(s.namespace).List(selector)
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

	obj, err := verifyCreation(ret.Name, "node", func(name string) (runtime.Object, error) {
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
			Name:                     name,
			AllowScheduling:          true,
			EvictionRequested:        false,
			Tags:                     []string{},
			EngineManagerCPURequest:  0,
			ReplicaManagerCPURequest: 0,
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
		disks, err := types.CreateDefaultDisk(dataPath)
		if err != nil {
			return nil, err
		}
		node.Spec.Disks = disks
	}

	return s.CreateNode(node)
}

func (s *DataStore) GetNodeRO(name string) (*longhorn.Node, error) {
	return s.nLister.Nodes(s.namespace).Get(name)
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
	nodes, err := s.ListReadyNodes()
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
	node, err := s.GetNode(nodeName)
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
	verifyUpdate(node.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(node.Name, obj, func(name string) (runtime.Object, error) {
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
	return s.nLister.Nodes(s.namespace).List(labels.Everything())
}

func (s *DataStore) ListNodesWithEngineImage(ei *longhorn.EngineImage) (map[string]*longhorn.Node, error) {
	nodes, err := s.ListNodes()
	if err != nil {
		return nil, err
	}
	for nodeID := range nodes {
		if !ei.Status.NodeDeploymentMap[nodeID] {
			delete(nodes, nodeID)
		}
	}
	return nodes, nil
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

// filterSchedulableNodes returns only the nodes that are ready
func filterSchedulableNodes(nodes map[string]*longhorn.Node) map[string]*longhorn.Node {
	return filterNodes(nodes, func(node *longhorn.Node) bool {
		nodeSchedulableCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeSchedulable)
		return nodeSchedulableCondition.Status == longhorn.ConditionStatusTrue
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

func (s *DataStore) ListReadyAndSchedulableNodes() (map[string]*longhorn.Node, error) {
	nodes, err := s.ListNodes()
	if err != nil {
		return nil, err
	}
	return filterSchedulableNodes(filterReadyNodes(nodes)), nil
}

// ListReadyNodesWithEngineImage returns list of ready nodes that have the corresponding engine image deploying or deployed
func (s *DataStore) ListReadyNodesWithEngineImage(image string) (map[string]*longhorn.Node, error) {
	ei, err := s.GetEngineImage(types.GetEngineImageChecksumName(image))
	if err != nil {
		return nil, fmt.Errorf("unable to get engine image %v: %v", image, err)
	}
	if ei.Status.State != longhorn.EngineImageStateDeployed && ei.Status.State != longhorn.EngineImageStateDeploying {
		return map[string]*longhorn.Node{}, nil
	}
	nodes, err := s.ListNodesWithEngineImage(ei)
	if err != nil {
		return nil, err
	}
	readyNodes := filterReadyNodes(nodes)
	return readyNodes, nil
}

// ListReadyNodesWithReadyEngineImage returns list of ready nodes that have the corresponding engine image deployed
func (s *DataStore) ListReadyNodesWithReadyEngineImage(image string) (map[string]*longhorn.Node, error) {
	ei, err := s.GetEngineImage(types.GetEngineImageChecksumName(image))
	if err != nil {
		return nil, fmt.Errorf("unable to get engine image %v: %v", image, err)
	}
	if ei.Status.State != longhorn.EngineImageStateDeployed {
		return map[string]*longhorn.Node{}, nil
	}
	nodes, err := s.ListNodesWithEngineImage(ei)
	if err != nil {
		return nil, err
	}
	readyNodes := filterReadyNodes(nodes)
	return readyNodes, nil
}

// GetRandomReadyNode gets a list of all Node in the given namespace and
// returns the first Node marked with condition ready and allow scheduling
func (s *DataStore) GetRandomReadyNode() (*longhorn.Node, error) {
	logrus.Debugf("Prepare to find a random ready node")
	nodesRO, err := s.ListNodesRO()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get random ready node")
	}

	for _, node := range nodesRO {
		readyCondition := types.GetCondition(node.Status.Conditions, longhorn.NodeConditionTypeReady)
		if readyCondition.Status == longhorn.ConditionStatusTrue && node.Spec.AllowScheduling {
			return node.DeepCopy(), nil
		}
	}

	return nil, fmt.Errorf("unable to get a ready node")
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
	return s.rLister.Replicas(s.namespace).List(backingImageSelector)
}

// ListReplicasByNodeRO returns a list of all Replicas on node Name for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListReplicasByNodeRO(name string) ([]*longhorn.Replica, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.rLister.Replicas(s.namespace).List(nodeSelector)
}

func tagNodeLabel(nodeID string, obj runtime.Object) error {
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

func tagDiskUUIDLabel(diskUUID string, obj runtime.Object) error {
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

// tagLonghornNodeLabel fixes the new label `longhorn.io/node` for object.
// It can replace func `tagNodeLabel` if the old label `longhornnode` is
// deprecated.
func tagLonghornNodeLabel(nodeID string, obj runtime.Object) error {
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

// tagLonghornDiskUUIDLabel fixes the new label `longhorn.io/disk-uuid` for
// object. It can replace func `tagDiskUUIDLabel` if the old label
// `longhorndiskuuid` is deprecated.
func tagLonghornDiskUUIDLabel(diskUUID string, obj runtime.Object) error {
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

func tagBackingImageLabel(backingImageName string, obj runtime.Object) error {
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

func tagBackupVolumeLabel(backupVolumeName string, obj runtime.Object) error {
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

func FixupRecurringJob(v *longhorn.Volume) error {
	if err := tagRecurringJobDefaultLabel(v); err != nil {
		return err
	}
	return nil
}

func tagRecurringJobDefaultLabel(obj runtime.Object) error {
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
	settings, err := s.GetSetting(settingName)
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
	settings, err := s.GetSetting(settingName)
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
	ipp, err := s.GetSetting(types.SettingNameSystemManagedPodsImagePullPolicy)
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
	setting, err := s.GetSetting(types.SettingNameTaintToleration)
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
	setting, err := s.GetSetting(types.SettingNameSystemManagedComponentsNodeSelector)
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
	e.Status.RestoreStatus = nil
	e.Status.PurgeStatus = nil
	e.Status.RebuildStatus = nil
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
	engineList, err := s.eLister.Engines(s.namespace).List(nodeSelector)
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

	obj, err := verifyCreation(ret.Name, "instance manager", func(name string) (runtime.Object, error) {
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
	return s.imLister.InstanceManagers(s.namespace).Get(name)
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

// GetDefaultEngineInstanceManagerByNode returns the given node's engine InstanceManager
// that is using the default instance manager image.
func (s *DataStore) GetDefaultEngineInstanceManagerByNode(name string) (*longhorn.InstanceManager, error) {
	defaultInstanceManagerImage, err := s.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
	if err != nil {
		return nil, err
	}

	engineIMs, err := s.ListInstanceManagersBySelector(name, defaultInstanceManagerImage, longhorn.InstanceManagerTypeEngine)
	if err != nil {
		return nil, err
	}

	engineIM := &longhorn.InstanceManager{}
	for _, im := range engineIMs {
		engineIM = im

		if len(engineIMs) != 1 {
			logrus.Warnf("Found more than 1 %v instance manager with %v on %v, use %v", longhorn.InstanceManagerTypeEngine, defaultInstanceManagerImage, name, engineIM.Name)
			break
		}
	}

	return engineIM, nil
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
	}

	return longhorn.InstanceManagerType(""), fmt.Errorf("unknown type %v for instance manager %v", imType, im.Name)
}

// ListInstanceManagersBySelector gets a list of InstanceManager by labels for
// the given namespace. Returns an object contains all InstanceManager
func (s *DataStore) ListInstanceManagersBySelector(node, instanceManagerImage string, managerType longhorn.InstanceManagerType) (map[string]*longhorn.InstanceManager, error) {
	itemMap := map[string]*longhorn.InstanceManager{}

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetInstanceManagerLabels(node, instanceManagerImage, managerType),
	})
	if err != nil {
		return nil, err
	}

	listRO, err := s.imLister.InstanceManagers(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}
	for _, itemRO := range listRO {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// GetInstanceManagerByInstance gets a list of InstanceManager for the given
// object. Returns error if more than one InstanceManager is found
func (s *DataStore) GetInstanceManagerByInstance(obj interface{}) (*longhorn.InstanceManager, error) {
	var (
		name, nodeID string
		imType       longhorn.InstanceManagerType
	)

	image, err := s.GetSettingValueExisted(types.SettingNameDefaultInstanceManagerImage)
	if err != nil {
		return nil, err
	}

	switch obj.(type) {
	case *longhorn.Engine:
		engine := obj.(*longhorn.Engine)
		name = engine.Name
		nodeID = engine.Spec.NodeID
		imType = longhorn.InstanceManagerTypeEngine
	case *longhorn.Replica:
		replica := obj.(*longhorn.Replica)
		name = replica.Name
		nodeID = replica.Spec.NodeID
		imType = longhorn.InstanceManagerTypeReplica
	default:
		return nil, fmt.Errorf("unknown type for GetInstanceManagerByInstance, %+v", obj)
	}
	if nodeID == "" {
		return nil, fmt.Errorf("invalid request for GetInstanceManagerByInstance: no NodeID specified for instance %v", name)
	}

	imMap, err := s.ListInstanceManagersBySelector(nodeID, image, imType)
	if err != nil {
		return nil, err
	}
	if len(imMap) == 1 {
		for _, im := range imMap {
			return im, nil
		}

	}
	return nil, fmt.Errorf("cannot find the only available instance manager for instance %v, node %v, instance manager image %v, type %v", name, nodeID, image, imType)
}

// ListInstanceManagersByNode returns ListInstanceManagersBySelector
func (s *DataStore) ListInstanceManagersByNode(node string, imType longhorn.InstanceManagerType) (map[string]*longhorn.InstanceManager, error) {
	return s.ListInstanceManagersBySelector(node, "", imType)
}

// ListInstanceManagers gets a list of InstanceManagers for the given namespace.
// Returns a new InstanceManager object
func (s *DataStore) ListInstanceManagers() (map[string]*longhorn.InstanceManager, error) {
	itemMap := map[string]*longhorn.InstanceManager{}

	list, err := s.imLister.InstanceManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// UpdateInstanceManager updates Longhorn InstanceManager resource and verifies update
func (s *DataStore) UpdateInstanceManager(im *longhorn.InstanceManager) (*longhorn.InstanceManager, error) {
	obj, err := s.lhClient.LonghornV1beta2().InstanceManagers(s.namespace).Update(context.TODO(), im, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(im.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(im.Name, obj, func(name string) (runtime.Object, error) {
		return s.GetInstanceManagerRO(name)
	})
	return obj, nil
}

func verifyCreation(name, kind string, getMethod func(name string) (runtime.Object, error)) (runtime.Object, error) {
	// WORKAROUND: The immedidate read after object's creation can fail.
	// See https://github.com/longhorn/longhorn/issues/133
	var (
		ret runtime.Object
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
		return nil, fmt.Errorf("unable to verify the existence of newly created %s %s: %v", kind, name, err)
	}
	return ret, nil
}

func verifyUpdate(name string, obj runtime.Object, getMethod func(name string) (runtime.Object, error)) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		logrus.Errorf("BUG: datastore: cannot verify update for %v (%+v) because cannot get accessor: %v", name, obj, err)
		return
	}
	minimalResourceVersion := accessor.GetResourceVersion()
	verified := false
	for i := 0; i < VerificationRetryCounts; i++ {
		ret, err := getMethod(name)
		if err != nil {
			logrus.Errorf("datastore: failed to get updated object %v", name)
			return
		}
		accessor, err := meta.Accessor(ret)
		if err != nil {
			logrus.Errorf("BUG: datastore: cannot verify update for %v because cannot get accessor for updated object: %v", name, err)
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
		logrus.Errorf("datastore: failed to parse current resource version %v: %v", curr, err)
		return false
	}
	minVersion, err := strconv.ParseInt(min, 10, 64)
	if err != nil {
		logrus.Errorf("datastore: failed to parse minimal resource version %v: %v", min, err)
		return false
	}
	return currVersion >= minVersion
}

// IsEngineImageCLIAPIVersionOne get engine image CLIAPIVersion for the given name.
// Returns true if CLIAPIVersion is 1
func (s *DataStore) IsEngineImageCLIAPIVersionOne(imageName string) (bool, error) {
	version, err := s.GetEngineImageCLIAPIVersion(imageName)
	if err != nil {
		return false, err
	}

	if version == 1 {
		return true, nil
	}
	return false, nil
}

// GetEngineImageCLIAPIVersion get engine image for the given name and returns the
// CLIAPIVersion
func (s *DataStore) GetEngineImageCLIAPIVersion(imageName string) (int, error) {
	if imageName == "" {
		return -1, fmt.Errorf("cannot check the CLI API Version based on empty image name")
	}
	ei, err := s.GetEngineImage(types.GetEngineImageChecksumName(imageName))
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

	obj, err := verifyCreation(ret.Name, "share manager", func(name string) (runtime.Object, error) {
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
	if err := util.AddFinalizer(longhornFinalizerKey, sm); err != nil {
		return nil, err
	}

	obj, err := s.lhClient.LonghornV1beta2().ShareManagers(s.namespace).Update(context.TODO(), sm, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(sm.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(sm.Name, obj, func(name string) (runtime.Object, error) {
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
	return s.smLister.ShareManagers(s.namespace).Get(name)
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

	list, err := s.smLister.ShareManagers(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
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

	obj, err := verifyCreation(ret.Name, "backup target", func(name string) (runtime.Object, error) {
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

// ListBackupTargets returns an object contains all backup targets in the cluster BackupTargets CR
func (s *DataStore) ListBackupTargets() (map[string]*longhorn.BackupTarget, error) {
	list, err := s.btLister.BackupTargets(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupTarget{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// GetBackupTargetRO returns the BackupTarget with the given backup target name in the cluster
func (s *DataStore) GetBackupTargetRO(backupTargetName string) (*longhorn.BackupTarget, error) {
	return s.btLister.BackupTargets(s.namespace).Get(backupTargetName)
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
	obj, err := s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Update(context.TODO(), backupTarget, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(backupTarget.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(backupTarget.Name, obj, func(name string) (runtime.Object, error) {
		return s.GetBackupTargetRO(name)
	})
	return obj, nil
}

// DeleteBackupTarget won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteBackupTarget(backupTargetName string) error {
	return s.lhClient.LonghornV1beta2().BackupTargets(s.namespace).Delete(context.TODO(), backupTargetName, metav1.DeleteOptions{})
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

	obj, err := verifyCreation(ret.Name, "backup volume", func(name string) (runtime.Object, error) {
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
	list, err := s.bvLister.BackupVolumes(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	itemMap := map[string]*longhorn.BackupVolume{}
	for _, itemRO := range list {
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func getBackupVolumeSelector(backupVolumeName string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackupVolumeLabels(backupVolumeName),
	})
}

// GetBackupVolumeRO returns the BackupVolume with the given backup volume name in the cluster
func (s *DataStore) GetBackupVolumeRO(backupVolumeName string) (*longhorn.BackupVolume, error) {
	return s.bvLister.BackupVolumes(s.namespace).Get(backupVolumeName)
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
	verifyUpdate(backupVolume.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(backupVolume.Name, obj, func(name string) (runtime.Object, error) {
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
	if err := tagBackupVolumeLabel(backupVolumeName, backup); err != nil {
		return nil, err
	}
	ret, err := s.lhClient.LonghornV1beta2().Backups(s.namespace).Create(context.TODO(), backup, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	if SkipListerCheck {
		return ret, nil
	}

	obj, err := verifyCreation(ret.Name, "backup", func(name string) (runtime.Object, error) {
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

// ListBackupsWithBackupVolumeName returns an object contains all backups in the cluster Backups CR
// of the given backup volume name
func (s *DataStore) ListBackupsWithBackupVolumeName(backupVolumeName string) (map[string]*longhorn.Backup, error) {
	selector, err := getBackupVolumeSelector(backupVolumeName)
	if err != nil {
		return nil, err
	}

	list, err := s.bLister.Backups(s.namespace).List(selector)
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
	return s.bLister.Backups(s.namespace).List(labels.Everything())
}

// ListBackups returns an object contains all backups in the cluster Backups CR
func (s *DataStore) ListBackups() (map[string]*longhorn.Backup, error) {
	list, err := s.bLister.Backups(s.namespace).List(labels.Everything())
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
	return s.bLister.Backups(s.namespace).Get(backupName)
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
	verifyUpdate(backup.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(backup.Name, obj, func(name string) (runtime.Object, error) {
		return s.GetBackupRO(name)
	})
	return obj, nil
}

// DeleteBackup won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteBackup(backupName string) error {
	return s.lhClient.LonghornV1beta2().Backups(s.namespace).Delete(context.TODO(), backupName, metav1.DeleteOptions{})
}

// DeleteAllBackupsForBackupVolume won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteAllBackupsForBackupVolume(backupVolumeName string) error {
	return s.lhClient.LonghornV1beta2().Backups(s.namespace).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", types.LonghornLabelBackupVolume, backupVolumeName)})
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

	obj, err := verifyCreation(snapshot.Name, "snapshot", func(name string) (runtime.Object, error) {
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
	return s.snapLister.Snapshots(s.namespace).Get(snapName)
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
	verifyUpdate(snap.Name, obj, func(name string) (runtime.Object, error) {
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
	list, err := s.snapLister.Snapshots(s.namespace).List(selector)
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
	list, err := s.snapLister.Snapshots(s.namespace).List(labels.Everything())
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

	obj, err := verifyCreation(ret.Name, "recurring job", func(name string) (runtime.Object, error) {
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

	list, err := s.rjLister.RecurringJobs(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

func (s *DataStore) getRecurringJobRO(name string) (*longhorn.RecurringJob, error) {
	return s.rjLister.RecurringJobs(s.namespace).Get(name)
}

// GetRecurringJob gets the RecurringJob for the given name and namespace.
// Returns a mutable RecurringJob object
func (s *DataStore) GetRecurringJob(name string) (*longhorn.RecurringJob, error) {
	result, err := s.getRecurringJob(name)
	if err != nil {
		return nil, err
	}
	return result.DeepCopy(), nil
}

func (s *DataStore) getRecurringJob(name string) (*longhorn.RecurringJob, error) {
	return s.rjLister.RecurringJobs(s.namespace).Get(name)
}

// UpdateRecurringJob updates Longhorn RecurringJob and verifies update
func (s *DataStore) UpdateRecurringJob(recurringJob *longhorn.RecurringJob) (*longhorn.RecurringJob, error) {
	obj, err := s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).Update(context.TODO(), recurringJob, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	verifyUpdate(recurringJob.Name, obj, func(name string) (runtime.Object, error) {
		return s.getRecurringJob(name)
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
	verifyUpdate(recurringJob.Name, obj, func(name string) (runtime.Object, error) {
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
	if job.Cron == "" || job.Task == "" || job.Name == "" || job.Retain == 0 {
		return fmt.Errorf("invalid job %+v", job)
	}
	if job.Task != longhorn.RecurringJobTypeBackup && job.Task != longhorn.RecurringJobTypeSnapshot {
		return fmt.Errorf("recurring job type %v is not valid", job.Task)
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
		if !util.ValidateName(group) {
			return fmt.Errorf("invalid group name %v", group)
		}
	}
	if job.Labels != nil {
		if _, err := util.ValidateSnapshotLabels(job.Labels); err != nil {
			return err
		}
	}
	return nil
}

func ValidateRecurringJobs(jobs []longhorn.RecurringJobSpec) error {
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

	if totalJobRetainCount > MaxRecurringJobRetain {
		return fmt.Errorf("job Can't retain more than %d snapshots", MaxRecurringJobRetain)
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

	obj, err := verifyCreation(ret.Name, "orphan", func(name string) (runtime.Object, error) {
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
	return s.oLister.Orphans(s.namespace).Get(orphanName)
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
	verifyUpdate(orphan.Name, obj, func(name string) (runtime.Object, error) {
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
	verifyUpdate(orphan.Name, obj, func(name string) (runtime.Object, error) {
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
	list, err := s.oLister.Orphans(s.namespace).List(selector)
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
	return s.oLister.Orphans(s.namespace).List(labels.Everything())
}

// ListOrphansByNodeRO returns a list of all Orphans on node Name for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListOrphansByNodeRO(name string) ([]*longhorn.Orphan, error) {
	nodeSelector, err := getNodeSelector(name)
	if err != nil {
		return nil, err
	}
	return s.oLister.Orphans(s.namespace).List(nodeSelector)
}

// DeleteOrphan won't result in immediately deletion since finalizer was set by default
func (s *DataStore) DeleteOrphan(orphanName string) error {
	return s.lhClient.LonghornV1beta2().Orphans(s.namespace).Delete(context.TODO(), orphanName, metav1.DeleteOptions{})
}

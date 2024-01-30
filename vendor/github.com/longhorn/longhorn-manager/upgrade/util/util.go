package util

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/mod/semver"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/labels"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

const (
	// LonghornV1ToV2MinorVersionNum v1 minimal minor version when the upgrade path is from v1.x to v2.0
	// TODO: decide the v1 minimum version that could be upgraded to v2.0
	LonghornV1ToV2MinorVersionNum = 30
)

type ProgressMonitor struct {
	description                 string
	targetValue                 int
	currentValue                int
	currentProgressInPercentage float64
	mutex                       *sync.RWMutex
}

func NewProgressMonitor(description string, currentValue, targetValue int) *ProgressMonitor {
	pm := &ProgressMonitor{
		description:                 description,
		targetValue:                 targetValue,
		currentValue:                currentValue,
		currentProgressInPercentage: math.Floor(float64(currentValue*100) / float64(targetValue)),
		mutex:                       &sync.RWMutex{},
	}
	pm.logCurrentProgress()
	return pm
}

func (pm *ProgressMonitor) logCurrentProgress() {
	logrus.Infof("%v: current progress %v%% (%v/%v)", pm.description, pm.currentProgressInPercentage, pm.currentValue, pm.targetValue)
}

func (pm *ProgressMonitor) Inc() int {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	oldProgressInPercentage := pm.currentProgressInPercentage

	pm.currentValue++
	pm.currentProgressInPercentage = math.Floor(float64(pm.currentValue*100) / float64(pm.targetValue))
	if pm.currentProgressInPercentage != oldProgressInPercentage {
		pm.logCurrentProgress()
	}
	return pm.currentValue
}

func (pm *ProgressMonitor) SetCurrentValue(newValue int) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	oldProgressInPercentage := pm.currentProgressInPercentage

	pm.currentValue = newValue
	pm.currentProgressInPercentage = math.Floor(float64(pm.currentValue*100) / float64(pm.targetValue))
	if pm.currentProgressInPercentage != oldProgressInPercentage {
		pm.logCurrentProgress()
	}
}

func (pm *ProgressMonitor) GetCurrentProgress() (int, int, float64) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.currentValue, pm.targetValue, pm.currentProgressInPercentage
}

func ListShareManagerPods(namespace string, kubeClient *clientset.Clientset) ([]corev1.Pod, error) {
	smPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetShareManagerComponentLabel()).String(),
	})
	if err != nil {
		return nil, err
	}
	return smPodsList.Items, nil
}

func ListIMPods(namespace string, kubeClient *clientset.Clientset) ([]corev1.Pod, error) {
	imPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", types.GetLonghornLabelComponentKey(), types.LonghornLabelInstanceManager),
	})
	if err != nil {
		return nil, err
	}
	return imPodsList.Items, nil
}

func ListManagerPods(namespace string, kubeClient *clientset.Clientset) ([]corev1.Pod, error) {
	managerPodsList, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetManagerLabels()).String(),
	})
	if err != nil {
		return nil, err
	}
	return managerPodsList.Items, nil
}

func MergeStringMaps(baseMap, overwriteMap map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range baseMap {
		result[k] = v
	}
	for k, v := range overwriteMap {
		result[k] = v
	}
	return result
}

func GetCurrentLonghornVersion(namespace string, lhClient lhclientset.Interface) (string, error) {
	currentLHVersionSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameCurrentLonghornVersion), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return currentLHVersionSetting.Value, nil
}

func CreateOrUpdateLonghornVersionSetting(namespace string, lhClient *lhclientset.Clientset) error {
	s, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameCurrentLonghornVersion), metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		s = &longhorn.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(types.SettingNameCurrentLonghornVersion),
			},
			Value: meta.Version,
		}
		_, err := lhClient.LonghornV1beta2().Settings(namespace).Create(context.TODO(), s, metav1.CreateOptions{})
		return err
	}

	if s.Value != meta.Version {
		s.Value = meta.Version
		if s.Annotations == nil {
			s.Annotations = make(map[string]string)
		}
		s.Annotations[types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn)] = ""
		s, err = lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), s, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		delete(s.Annotations, types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn))
		_, err = lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), s, metav1.UpdateOptions{})
		return err
	}
	return nil
}

// CheckUpgradePathSupported returns if the upgrade path from lhCurrentVersion to meta.Version is supported.
//
//	For example: upgrade path is from x.y.z to a.b.c,
//	0 <= a-x <= 1 is supported, and y should be after a specific version if a-x == 1
//	0 <= b-y <= 1 is supported when a-x == 0
//	all downgrade is not supported
func CheckUpgradePathSupported(namespace string, lhClient lhclientset.Interface) error {
	lhCurrentVersion, err := GetCurrentLonghornVersion(namespace, lhClient)
	if err != nil {
		return err
	}

	if lhCurrentVersion == "" {
		return nil
	}

	logrus.Infof("Checking if the upgrade path from %v to %v is supported", lhCurrentVersion, meta.Version)

	if !semver.IsValid(meta.Version) {
		return fmt.Errorf("failed to upgrade since upgrading version %v is not valid", meta.Version)
	}

	lhNewMajorVersion := semver.Major(meta.Version)
	lhCurrentMajorVersion := semver.Major(lhCurrentVersion)

	lhNewMajorVersionNum, lhNewMinorVersionNum, err := getMajorMinorInt(meta.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse upgrading %v major/minor version", meta.Version)
	}

	lhCurrentMajorVersionNum, lhCurrentMinorVersionNum, err := getMajorMinorInt(lhCurrentVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to parse current %v major/minor version", meta.Version)
	}

	if semver.Compare(lhCurrentMajorVersion, lhNewMajorVersion) > 0 {
		return fmt.Errorf("failed to upgrade since downgrading from %v to %v for major version is not supported", lhCurrentVersion, meta.Version)
	}

	if semver.Compare(lhCurrentMajorVersion, lhNewMajorVersion) < 0 {
		if (lhNewMajorVersionNum - lhCurrentMajorVersionNum) > 1 {
			return fmt.Errorf("failed to upgrade since upgrading from %v to %v for major version is not supported", lhCurrentVersion, meta.Version)
		}
		if lhCurrentMinorVersionNum < LonghornV1ToV2MinorVersionNum {
			return fmt.Errorf("failed to upgrade since upgrading major version with minor version under %v is not supported", LonghornV1ToV2MinorVersionNum)
		}
		return nil
	}

	if (lhNewMinorVersionNum - lhCurrentMinorVersionNum) > 1 {
		return fmt.Errorf("failed to upgrade since upgrading from %v to %v for minor version is not supported", lhCurrentVersion, meta.Version)
	}

	if (lhNewMinorVersionNum - lhCurrentMinorVersionNum) == 1 {
		return nil
	}

	if semver.Compare(lhCurrentVersion, meta.Version) > 0 {
		return fmt.Errorf("failed to upgrade since downgrading from %v to %v is not supported", lhCurrentVersion, meta.Version)
	}

	return nil
}

func DeleteRemovedSettings(namespace string, lhClient *lhclientset.Clientset) error {
	isKnownSetting := func(knownSettingNames []types.SettingName, name types.SettingName) bool {
		for _, knownSettingName := range knownSettingNames {
			if name == knownSettingName {
				return true
			}
		}
		return false
	}

	existingSettingList, err := lhClient.LonghornV1beta2().Settings(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, existingSetting := range existingSettingList.Items {
		if !isKnownSetting(types.SettingNameList, types.SettingName(existingSetting.Name)) {
			logrus.Infof("Deleting removed setting: %v", existingSetting.Name)
			if err = lhClient.LonghornV1beta2().Settings(namespace).Delete(context.TODO(), existingSetting.Name, metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func getMajorMinorInt(v string) (int, int, error) {
	majorNum, err := getMajorInt(v)
	if err != nil {
		return -1, -1, err
	}
	minorNum, err := getMinorInt(v)
	if err != nil {
		return -1, -1, err
	}
	return majorNum, minorNum, nil
}

func getMajorInt(v string) (int, error) {
	versionStr := removeFirstChar(v)
	return strconv.Atoi(strings.Split(versionStr, ".")[0])
}

func getMinorInt(v string) (int, error) {
	versionStr := removeFirstChar(v)
	return strconv.Atoi(strings.Split(versionStr, ".")[1])
}

func removeFirstChar(v string) string {
	if v[0] == 'v' {
		return v[1:]
	}
	return v
}

// ListAndUpdateSettingsInProvidedCache list all settings and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateSettingsInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Setting, error) {
	if v, ok := resourceMaps[types.LonghornKindSetting]; ok {
		return v.(map[string]*longhorn.Setting), nil
	}

	settings := map[string]*longhorn.Setting{}
	settingList, err := lhClient.LonghornV1beta2().Settings(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, setting := range settingList.Items {
		settingCopy := longhorn.Setting{}
		if err := copier.Copy(&settingCopy, setting); err != nil {
			return nil, err
		}
		settings[setting.Name] = &settingCopy
	}

	resourceMaps[types.LonghornKindSetting] = settings

	return settings, nil
}

// ListAndUpdateNodesInProvidedCache list all nodes and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateNodesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Node, error) {
	if v, ok := resourceMaps[types.LonghornKindNode]; ok {
		return v.(map[string]*longhorn.Node), nil
	}

	nodes := map[string]*longhorn.Node{}
	nodeList, err := lhClient.LonghornV1beta2().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, node := range nodeList.Items {
		nodes[node.Name] = &nodeList.Items[i]
	}

	resourceMaps[types.LonghornKindNode] = nodes

	return nodes, nil
}

// ListAndUpdateOrphansInProvidedCache list all orphans and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateOrphansInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Orphan, error) {
	if v, ok := resourceMaps[types.LonghornKindOrphan]; ok {
		return v.(map[string]*longhorn.Orphan), nil
	}

	orphans := map[string]*longhorn.Orphan{}
	orphanList, err := lhClient.LonghornV1beta2().Orphans(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, orphan := range orphanList.Items {
		orphans[orphan.Name] = &orphanList.Items[i]
	}

	resourceMaps[types.LonghornKindOrphan] = orphans

	return orphans, nil
}

// ListAndUpdateInstanceManagersInProvidedCache list all instanceManagers and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateInstanceManagersInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.InstanceManager, error) {
	if v, ok := resourceMaps[types.LonghornKindInstanceManager]; ok {
		return v.(map[string]*longhorn.InstanceManager), nil
	}

	ims := map[string]*longhorn.InstanceManager{}
	imList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, im := range imList.Items {
		ims[im.Name] = &imList.Items[i]
	}

	resourceMaps[types.LonghornKindInstanceManager] = ims

	return ims, nil
}

// ListAndUpdateVolumesInProvidedCache list all volumes and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateVolumesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Volume, error) {
	if v, ok := resourceMaps[types.LonghornKindVolume]; ok {
		return v.(map[string]*longhorn.Volume), nil
	}

	volumes := map[string]*longhorn.Volume{}
	volumeList, err := lhClient.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, volume := range volumeList.Items {
		volumes[volume.Name] = &volumeList.Items[i]
	}

	resourceMaps[types.LonghornKindVolume] = volumes

	return volumes, nil
}

// ListAndUpdateReplicasInProvidedCache list all replicas and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateReplicasInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Replica, error) {
	if v, ok := resourceMaps[types.LonghornKindReplica]; ok {
		return v.(map[string]*longhorn.Replica), nil
	}

	replicas := map[string]*longhorn.Replica{}
	replicaList, err := lhClient.LonghornV1beta2().Replicas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, replica := range replicaList.Items {
		replicas[replica.Name] = &replicaList.Items[i]
	}

	resourceMaps[types.LonghornKindReplica] = replicas

	return replicas, nil
}

// ListAndUpdateEnginesInProvidedCache list all engines and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateEnginesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Engine, error) {
	if v, ok := resourceMaps[types.LonghornKindEngine]; ok {
		return v.(map[string]*longhorn.Engine), nil
	}

	engines := map[string]*longhorn.Engine{}
	engineList, err := lhClient.LonghornV1beta2().Engines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, engine := range engineList.Items {
		engines[engine.Name] = &engineList.Items[i]
	}

	resourceMaps[types.LonghornKindEngine] = engines

	return engines, nil
}

// ListAndUpdateBackupsInProvidedCache list all backups and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateBackupsInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Backup, error) {
	if v, ok := resourceMaps[types.LonghornKindBackup]; ok {
		return v.(map[string]*longhorn.Backup), nil
	}

	backups := map[string]*longhorn.Backup{}
	backupList, err := lhClient.LonghornV1beta2().Backups(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, backup := range backupList.Items {
		backups[backup.Name] = &backupList.Items[i]
	}

	resourceMaps[types.LonghornKindBackup] = backups

	return backups, nil
}

// ListAndUpdateSnapshotsInProvidedCache list all snapshots and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateSnapshotsInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.Snapshot, error) {
	if v, ok := resourceMaps[types.LonghornKindSnapshot]; ok {
		return v.(map[string]*longhorn.Snapshot), nil
	}

	snapshots := map[string]*longhorn.Snapshot{}
	snapshotList, err := lhClient.LonghornV1beta2().Snapshots(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, snapshot := range snapshotList.Items {
		snapshots[snapshot.Name] = &snapshotList.Items[i]
	}

	resourceMaps[types.LonghornKindSnapshot] = snapshots

	return snapshots, nil
}

// ListAndUpdateEngineImagesInProvidedCache list all engineImages and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateEngineImagesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.EngineImage, error) {
	if v, ok := resourceMaps[types.LonghornKindEngineImage]; ok {
		return v.(map[string]*longhorn.EngineImage), nil
	}

	eis := map[string]*longhorn.EngineImage{}
	eiList, err := lhClient.LonghornV1beta2().EngineImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, ei := range eiList.Items {
		eis[ei.Name] = &eiList.Items[i]
	}

	resourceMaps[types.LonghornKindEngineImage] = eis

	return eis, nil
}

// ListAndUpdateShareManagersInProvidedCache list all shareManagers and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateShareManagersInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.ShareManager, error) {
	if v, ok := resourceMaps[types.LonghornKindShareManager]; ok {
		return v.(map[string]*longhorn.ShareManager), nil
	}

	sms := map[string]*longhorn.ShareManager{}
	smList, err := lhClient.LonghornV1beta2().ShareManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, sm := range smList.Items {
		sms[sm.Name] = &smList.Items[i]
	}

	resourceMaps[types.LonghornKindShareManager] = sms

	return sms, nil
}

// ListAndUpdateBackingImagesInProvidedCache list all backingImages and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateBackingImagesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.BackingImage, error) {
	if v, ok := resourceMaps[types.LonghornKindBackingImage]; ok {
		return v.(map[string]*longhorn.BackingImage), nil
	}

	bis := map[string]*longhorn.BackingImage{}
	biList, err := lhClient.LonghornV1beta2().BackingImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, bi := range biList.Items {
		bis[bi.Name] = &biList.Items[i]
	}

	resourceMaps[types.LonghornKindBackingImage] = bis

	return bis, nil
}

// ListAndUpdateBackingImageDataSourcesInProvidedCache list all backingImageDataSources and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateBackingImageDataSourcesInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.BackingImageDataSource, error) {
	if v, ok := resourceMaps[types.LonghornKindBackingImageDataSource]; ok {
		return v.(map[string]*longhorn.BackingImageDataSource), nil
	}

	bidss := map[string]*longhorn.BackingImageDataSource{}
	bidsList, err := lhClient.LonghornV1beta2().BackingImageDataSources(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, bids := range bidsList.Items {
		bidss[bids.Name] = &bidsList.Items[i]
	}

	resourceMaps[types.LonghornKindBackingImageDataSource] = bidss

	return bidss, nil
}

// ListAndUpdateRecurringJobsInProvidedCache list all recurringJobs and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateRecurringJobsInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.RecurringJob, error) {
	if v, ok := resourceMaps[types.LonghornKindRecurringJob]; ok {
		return v.(map[string]*longhorn.RecurringJob), nil
	}

	recurringJobs := map[string]*longhorn.RecurringJob{}
	recurringJobList, err := lhClient.LonghornV1beta2().RecurringJobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, recurringJob := range recurringJobList.Items {
		recurringJobs[recurringJob.Name] = &recurringJobList.Items[i]
	}

	resourceMaps[types.LonghornKindRecurringJob] = recurringJobs

	return recurringJobs, nil
}

// ListAndUpdateVolumeAttachmentsInProvidedCache list all volumeAttachments and save them into the provided cached `resourceMap`. This method is not thread-safe.
func ListAndUpdateVolumeAttachmentsInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) (map[string]*longhorn.VolumeAttachment, error) {
	if v, ok := resourceMaps[types.LonghornKindVolumeAttachment]; ok {
		return v.(map[string]*longhorn.VolumeAttachment), nil
	}

	volumeAttachments := map[string]*longhorn.VolumeAttachment{}
	volumeAttachmentList, err := lhClient.LonghornV1beta2().VolumeAttachments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i, volumeAttachment := range volumeAttachmentList.Items {
		volumeAttachments[volumeAttachment.Name] = &volumeAttachmentList.Items[i]
	}

	resourceMaps[types.LonghornKindVolumeAttachment] = volumeAttachments

	return volumeAttachments, nil
}

// CreateAndUpdateRecurringJobInProvidedCache creates a recurringJob and saves it into the provided cached `resourceMap`. This method is not thread-safe.
func CreateAndUpdateRecurringJobInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}, job *longhorn.RecurringJob) (*longhorn.RecurringJob, error) {
	obj, err := lhClient.LonghornV1beta2().RecurringJobs(namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		return obj, err
	}

	var recurringJobs map[string]*longhorn.RecurringJob
	if v, ok := resourceMaps[types.LonghornKindRecurringJob]; ok {
		recurringJobs = v.(map[string]*longhorn.RecurringJob)
	} else {
		recurringJobs = map[string]*longhorn.RecurringJob{}
	}
	recurringJobs[job.Name] = obj

	resourceMaps[types.LonghornKindRecurringJob] = recurringJobs

	return obj, nil
}

// CreateAndUpdateBackingImageInProvidedCache creates a backingImage and saves it into the provided cached `resourceMap`. This method is not thread-safe.
func CreateAndUpdateBackingImageInProvidedCache(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}, bid *longhorn.BackingImageDataSource) (*longhorn.BackingImageDataSource, error) {
	obj, err := lhClient.LonghornV1beta2().BackingImageDataSources(namespace).Create(context.TODO(), bid, metav1.CreateOptions{})
	if err != nil {
		return obj, err
	}

	var bids map[string]*longhorn.BackingImageDataSource
	if v, ok := resourceMaps[types.LonghornKindBackingImageDataSource]; ok {
		bids = v.(map[string]*longhorn.BackingImageDataSource)
	} else {
		bids = map[string]*longhorn.BackingImageDataSource{}
	}
	bids[bid.Name] = obj

	resourceMaps[types.LonghornKindBackingImageDataSource] = bids

	return obj, nil
}

// UpdateResources persists all the resources' spec changes in provided cached `resourceMap`. This method is not thread-safe.
func UpdateResources(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) error {
	var err error

	for resourceKind, resourceMap := range resourceMaps {
		switch resourceKind {
		case types.LonghornKindNode:
			err = updateNodes(namespace, lhClient, resourceMap.(map[string]*longhorn.Node))
		case types.LonghornKindVolume:
			err = updateVolumes(namespace, lhClient, resourceMap.(map[string]*longhorn.Volume))
		case types.LonghornKindEngine:
			err = updateEngines(namespace, lhClient, resourceMap.(map[string]*longhorn.Engine))
		case types.LonghornKindReplica:
			err = updateReplicas(namespace, lhClient, resourceMap.(map[string]*longhorn.Replica))
		case types.LonghornKindBackup:
			err = updateBackups(namespace, lhClient, resourceMap.(map[string]*longhorn.Backup))
		case types.LonghornKindEngineImage:
			err = updateEngineImages(namespace, lhClient, resourceMap.(map[string]*longhorn.EngineImage))
		case types.LonghornKindInstanceManager:
			err = updateInstanceManagers(namespace, lhClient, resourceMap.(map[string]*longhorn.InstanceManager))
		case types.LonghornKindShareManager:
			err = updateShareManagers(namespace, lhClient, resourceMap.(map[string]*longhorn.ShareManager))
		case types.LonghornKindBackingImage:
			err = updateBackingImages(namespace, lhClient, resourceMap.(map[string]*longhorn.BackingImage))
		case types.LonghornKindRecurringJob:
			err = updateRecurringJobs(namespace, lhClient, resourceMap.(map[string]*longhorn.RecurringJob))
		case types.LonghornKindSetting:
			err = updateSettings(namespace, lhClient, resourceMap.(map[string]*longhorn.Setting))
		case types.LonghornKindVolumeAttachment:
			err = createOrUpdateVolumeAttachments(namespace, lhClient, resourceMap.(map[string]*longhorn.VolumeAttachment))
		case types.LonghornKindSnapshot:
			err = updateSnapshots(namespace, lhClient, resourceMap.(map[string]*longhorn.Snapshot))
		case types.LonghornKindOrphan:
			err = updateOrphans(namespace, lhClient, resourceMap.(map[string]*longhorn.Orphan))
		default:
			return fmt.Errorf("resource kind %v is not able to updated", resourceKind)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func updateNodes(namespace string, lhClient *lhclientset.Clientset, nodes map[string]*longhorn.Node) error {
	existingNodeList, err := lhClient.LonghornV1beta2().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingNode := range existingNodeList.Items {
		node, ok := nodes[existingNode.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingNode.Spec, node.Spec) ||
			!reflect.DeepEqual(existingNode.ObjectMeta, node.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Nodes(namespace).Update(context.TODO(), node, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateVolumes(namespace string, lhClient *lhclientset.Clientset, volumes map[string]*longhorn.Volume) error {
	existingVolumeList, err := lhClient.LonghornV1beta2().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingVolume := range existingVolumeList.Items {
		volume, ok := volumes[existingVolume.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingVolume.Spec, volume.Spec) ||
			!reflect.DeepEqual(existingVolume.ObjectMeta, volume.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Volumes(namespace).Update(context.TODO(), volume, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateReplicas(namespace string, lhClient *lhclientset.Clientset, replicas map[string]*longhorn.Replica) error {
	existingReplicaList, err := lhClient.LonghornV1beta2().Replicas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingReplica := range existingReplicaList.Items {
		replica, ok := replicas[existingReplica.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingReplica.Spec, replica.Spec) ||
			!reflect.DeepEqual(existingReplica.ObjectMeta, replica.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Replicas(namespace).Update(context.TODO(), replica, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateEngines(namespace string, lhClient *lhclientset.Clientset, engines map[string]*longhorn.Engine) error {
	existingEngineList, err := lhClient.LonghornV1beta2().Engines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingEngine := range existingEngineList.Items {
		engine, ok := engines[existingEngine.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingEngine.Spec, engine.Spec) ||
			!reflect.DeepEqual(existingEngine.ObjectMeta, engine.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Engines(namespace).Update(context.TODO(), engine, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateBackups(namespace string, lhClient *lhclientset.Clientset, backups map[string]*longhorn.Backup) error {
	existingBackupList, err := lhClient.LonghornV1beta2().Backups(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, existingBackup := range existingBackupList.Items {
		backup, ok := backups[existingBackup.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingBackup.Spec, backup.Spec) ||
			!reflect.DeepEqual(existingBackup.ObjectMeta, backup.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Backups(namespace).Update(context.TODO(), backup, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateEngineImages(namespace string, lhClient *lhclientset.Clientset, engineImages map[string]*longhorn.EngineImage) error {
	existingEngineImageList, err := lhClient.LonghornV1beta2().EngineImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingEngineImage := range existingEngineImageList.Items {
		engineImage, ok := engineImages[existingEngineImage.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingEngineImage.Spec, engineImage.Spec) ||
			!reflect.DeepEqual(existingEngineImage.ObjectMeta, engineImage.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().EngineImages(namespace).Update(context.TODO(), engineImage, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateInstanceManagers(namespace string, lhClient *lhclientset.Clientset, instanceManagers map[string]*longhorn.InstanceManager) error {
	existingInstanceManagerList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingInstanceManager := range existingInstanceManagerList.Items {
		instanceManager, ok := instanceManagers[existingInstanceManager.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingInstanceManager.Spec, instanceManager.Spec) ||
			!reflect.DeepEqual(existingInstanceManager.ObjectMeta, instanceManager.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().InstanceManagers(namespace).Update(context.TODO(), instanceManager, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateShareManagers(namespace string, lhClient *lhclientset.Clientset, shareManagers map[string]*longhorn.ShareManager) error {
	existingShareManagerList, err := lhClient.LonghornV1beta2().ShareManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingShareManager := range existingShareManagerList.Items {
		shareManager, ok := shareManagers[existingShareManager.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingShareManager.Spec, shareManager.Spec) ||
			!reflect.DeepEqual(existingShareManager.ObjectMeta, shareManager.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().ShareManagers(namespace).Update(context.TODO(), shareManager, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateBackingImages(namespace string, lhClient *lhclientset.Clientset, backingImages map[string]*longhorn.BackingImage) error {
	existingBackingImagesList, err := lhClient.LonghornV1beta2().BackingImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingBackingImage := range existingBackingImagesList.Items {
		backingImage, ok := backingImages[existingBackingImage.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingBackingImage.Spec, backingImage.Spec) ||
			!reflect.DeepEqual(existingBackingImage.ObjectMeta, backingImage.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().BackingImages(namespace).Update(context.TODO(), backingImage, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateRecurringJobs(namespace string, lhClient *lhclientset.Clientset, recurringJobs map[string]*longhorn.RecurringJob) error {
	existingRecurringJobList, err := lhClient.LonghornV1beta2().RecurringJobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingRecurringJob := range existingRecurringJobList.Items {
		recurringJob, ok := recurringJobs[existingRecurringJob.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingRecurringJob.Spec, recurringJob.Spec) ||
			!reflect.DeepEqual(existingRecurringJob.ObjectMeta, recurringJob.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().RecurringJobs(namespace).Update(context.TODO(), recurringJob, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateSettings(namespace string, lhClient *lhclientset.Clientset, settings map[string]*longhorn.Setting) error {
	existingSettingList, err := lhClient.LonghornV1beta2().Settings(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, existingSetting := range existingSettingList.Items {
		setting, ok := settings[existingSetting.Name]
		if !ok {
			continue
		}

		if !reflect.DeepEqual(existingSetting.Value, setting.Value) {
			if setting.Annotations == nil {
				setting.Annotations = make(map[string]string)
			}
			setting.Annotations[types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn)] = ""
			setting, err = lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), setting, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			delete(setting.Annotations, types.GetLonghornLabelKey(types.UpdateSettingFromLonghorn))
			if _, err = lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), setting, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func updateSnapshots(namespace string, lhClient *lhclientset.Clientset, snapshots map[string]*longhorn.Snapshot) error {
	existingSnapshotList, err := lhClient.LonghornV1beta2().Snapshots(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingSnapshot := range existingSnapshotList.Items {
		snapshot, ok := snapshots[existingSnapshot.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingSnapshot.Spec, snapshot.Spec) ||
			!reflect.DeepEqual(existingSnapshot.ObjectMeta, snapshot.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Snapshots(namespace).Update(context.TODO(), snapshot, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateOrphans(namespace string, lhClient *lhclientset.Clientset, orphans map[string]*longhorn.Orphan) error {
	existingOrphanList, err := lhClient.LonghornV1beta2().Orphans(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingOrphan := range existingOrphanList.Items {
		orphan, ok := orphans[existingOrphan.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingOrphan.Spec, orphan.Spec) ||
			!reflect.DeepEqual(existingOrphan.ObjectMeta, orphan.ObjectMeta) {
			if _, err = lhClient.LonghornV1beta2().Orphans(namespace).Update(context.TODO(), orphan, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// VolumeAttachments are new in v1.5.0. v1.5.x upgrades must be able to create VolumeAttachments that don't exist (when
// coming from v1.4.x) or update VolumeAttachments that do exist (when coming from v1.5.x).
func createOrUpdateVolumeAttachments(namespace string, lhClient *lhclientset.Clientset, volumeAttachments map[string]*longhorn.VolumeAttachment) error {
	existingVolumeAttachmentList, err := lhClient.LonghornV1beta2().VolumeAttachments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	existingVolumeAttachmentMap := map[string]longhorn.VolumeAttachment{}
	for _, va := range existingVolumeAttachmentList.Items {
		existingVolumeAttachmentMap[va.Name] = va
	}

	for _, va := range volumeAttachments {
		if existingVolumeAttachment, ok := existingVolumeAttachmentMap[va.Name]; ok {
			if !reflect.DeepEqual(existingVolumeAttachment.Spec, va.Spec) ||
				!reflect.DeepEqual(existingVolumeAttachment.ObjectMeta, va.ObjectMeta) {
				if _, err = lhClient.LonghornV1beta2().VolumeAttachments(namespace).Update(context.TODO(), va, metav1.UpdateOptions{}); err != nil {
					return err
				}
			}
			continue
		}
		if _, err = lhClient.LonghornV1beta2().VolumeAttachments(namespace).Create(context.TODO(), va, metav1.CreateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// UpdateResourcesStatus persists all the resources' status changes in provided cached `resourceMap`. This method is not thread-safe.
func UpdateResourcesStatus(namespace string, lhClient *lhclientset.Clientset, resourceMaps map[string]interface{}) error {
	var err error

	for resourceKind, resourceMap := range resourceMaps {
		switch resourceKind {
		case types.LonghornKindNode:
			err = updateNodesStatus(namespace, lhClient, resourceMap.(map[string]*longhorn.Node))
		default:
			return fmt.Errorf("resource kind %v is not able to updated", resourceKind)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func updateNodesStatus(namespace string, lhClient *lhclientset.Clientset, nodes map[string]*longhorn.Node) error {
	existingNodeList, err := lhClient.LonghornV1beta2().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, existingNode := range existingNodeList.Items {
		node, ok := nodes[existingNode.Name]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(existingNode.Status, node.Status) {
			if _, err = lhClient.LonghornV1beta2().Nodes(namespace).UpdateStatus(context.TODO(), node, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

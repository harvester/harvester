package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	VersionTagLatest = "latest"
	VersionTagStable = "stable"
)

var (
	upgradeCheckInterval          = time.Hour
	settingControllerResyncPeriod = time.Hour
	checkUpgradeURL               = "https://longhorn-upgrade-responder.rancher.io/v1/checkupgrade"
)

type SettingController struct {
	*baseController

	namespace    string
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	// upgrade checker
	lastUpgradeCheckedTimestamp time.Time
	version                     string

	// backup store timer is responsible for updating the backupTarget.spec.syncRequestAt
	bsTimer *BackupStoreTimer
}

type BackupStoreTimer struct {
	logger       logrus.FieldLogger
	controllerID string
	ds           *datastore.DataStore

	pollInterval time.Duration
	stopCh       chan struct{}
}

type Version struct {
	Name        string // must be in semantic versioning
	ReleaseDate string
	Tags        []string
}

type CheckUpgradeRequest struct {
	AppVersion string            `json:"appVersion"`
	ExtraInfo  map[string]string `json:"extraInfo"`
}

type CheckUpgradeResponse struct {
	Versions []Version `json:"versions"`
}

func NewSettingController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace, controllerID, version string) *SettingController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	sc := &SettingController{
		baseController: newBaseController("longhorn-setting", logger),

		namespace:    namespace,
		controllerID: controllerID,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-setting-controller"}),

		ds: ds,

		version: version,
	}

	ds.SettingInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    sc.enqueueSetting,
		UpdateFunc: func(old, cur interface{}) { sc.enqueueSetting(cur) },
		DeleteFunc: sc.enqueueSetting,
	}, settingControllerResyncPeriod)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.SettingInformer.HasSynced)

	ds.NodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    sc.enqueueSettingForNode,
		UpdateFunc: func(old, cur interface{}) { sc.enqueueSettingForNode(cur) },
		DeleteFunc: sc.enqueueSettingForNode,
	}, 0)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.NodeInformer.HasSynced)

	ds.BackupTargetInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		DeleteFunc: sc.enqueueSettingForBackupTarget,
	}, 0)
	sc.cacheSyncs = append(sc.cacheSyncs, ds.BackupTargetInformer.HasSynced)

	return sc
}

func (sc *SettingController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer sc.queue.ShutDown()

	sc.logger.Info("Start Longhorn Setting controller")
	defer sc.logger.Info("Shutting down Longhorn Setting controller")

	if !cache.WaitForNamedCacheSync("longhorn settings", stopCh, sc.cacheSyncs...) {
		return
	}

	// must remain single threaded since backup store timer is not thread-safe now
	go wait.Until(sc.worker, time.Second, stopCh)

	<-stopCh
}

func (sc *SettingController) worker() {
	for sc.processNextWorkItem() {
	}
}

func (sc *SettingController) processNextWorkItem() bool {
	key, quit := sc.queue.Get()

	if quit {
		return false
	}
	defer sc.queue.Done(key)

	err := sc.syncSetting(key.(string))
	sc.handleErr(err, key)

	return true
}

func (sc *SettingController) handleErr(err error, key interface{}) {
	if err == nil {
		sc.queue.Forget(key)
		return
	}

	if sc.queue.NumRequeues(key) < maxRetries {
		sc.logger.WithError(err).Warnf("Error syncing Longhorn setting %v", key)
		sc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	sc.logger.WithError(err).Warnf("Dropping Longhorn setting %v out of the queue", key)
	sc.queue.Forget(key)
}

func (sc *SettingController) syncSetting(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync setting for %v", key)
	}()

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	switch name {
	case string(types.SettingNameUpgradeChecker):
		if err := sc.syncUpgradeChecker(); err != nil {
			return err
		}
	case string(types.SettingNameBackupTarget), string(types.SettingNameBackupTargetCredentialSecret), string(types.SettingNameBackupstorePollInterval):
		if err := sc.syncBackupTarget(); err != nil {
			return err
		}
	case string(types.SettingNameTaintToleration):
		if err := sc.updateTaintToleration(); err != nil {
			return err
		}
	case string(types.SettingNameSystemManagedComponentsNodeSelector):
		if err := sc.updateNodeSelector(); err != nil {
			return err
		}
	case string(types.SettingNameGuaranteedEngineManagerCPU):
	case string(types.SettingNameGuaranteedReplicaManagerCPU):
		if err := sc.updateInstanceManagerCPURequest(); err != nil {
			return err
		}
	case string(types.SettingNamePriorityClass):
		if err := sc.updatePriorityClass(); err != nil {
			return err
		}
	case string(types.SettingNameKubernetesClusterAutoscalerEnabled):
		if err := sc.updateKubernetesClusterAutoscalerEnabled(); err != nil {
			return err
		}
	case string(types.SettingNameStorageNetwork):
		if err := sc.updateCNI(); err != nil {
			return err
		}
	default:
	}

	return nil
}

// getResponsibleNodeID returns which node need to run
func getResponsibleNodeID(ds *datastore.DataStore) (string, error) {
	readyNodes, err := ds.ListReadyNodes()
	if err != nil {
		return "", err
	}
	if len(readyNodes) == 0 {
		return "", fmt.Errorf("no ready nodes available")
	}

	// We use the first node as the responsible node
	// If we pick a random node, there is probability
	// more than one node be responsible node at the same time
	var responsibleNodes []string
	for node := range readyNodes {
		responsibleNodes = append(responsibleNodes, node)
	}
	sort.Strings(responsibleNodes)
	return responsibleNodes[0], nil
}

func (sc *SettingController) syncBackupTarget() (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync backup target")
	}()

	stopTimer := func() {
		if sc.bsTimer != nil {
			sc.bsTimer.Stop()
			sc.bsTimer = nil
		}
	}

	responsibleNodeID, err := getResponsibleNodeID(sc.ds)
	if err != nil {
		sc.logger.WithError(err).Warn("Failed to select node for sync backup target")
		return err
	}
	if responsibleNodeID != sc.controllerID {
		stopTimer()
		return nil
	}

	// Get settings
	targetSetting, err := sc.ds.GetSetting(types.SettingNameBackupTarget)
	if err != nil {
		return err
	}

	secretSetting, err := sc.ds.GetSetting(types.SettingNameBackupTargetCredentialSecret)
	if err != nil {
		return err
	}

	interval, err := sc.ds.GetSettingAsInt(types.SettingNameBackupstorePollInterval)
	if err != nil {
		return err
	}
	pollInterval := time.Duration(interval) * time.Second

	backupTarget, err := sc.ds.GetBackupTarget(types.DefaultBackupTargetName)
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			return err
		}

		// Create the default BackupTarget CR if not present
		backupTarget, err = sc.ds.CreateBackupTarget(&longhorn.BackupTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: types.DefaultBackupTargetName,
			},
			Spec: longhorn.BackupTargetSpec{
				BackupTargetURL:  targetSetting.Value,
				CredentialSecret: secretSetting.Value,
				PollInterval:     metav1.Duration{Duration: pollInterval},
			},
		})
		if err != nil {
			sc.logger.WithError(err).Warn("Failed to create backup target")
			return err
		}
	}

	existingBackupTarget := backupTarget.DeepCopy()
	defer func() {
		backupTarget.Spec.BackupTargetURL = targetSetting.Value
		backupTarget.Spec.CredentialSecret = secretSetting.Value
		backupTarget.Spec.PollInterval = metav1.Duration{Duration: pollInterval}
		if !reflect.DeepEqual(existingBackupTarget.Spec, backupTarget.Spec) {
			// Force sync backup target once the BackupTarget spec be updated
			backupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
			if _, err = sc.ds.UpdateBackupTarget(backupTarget); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
				sc.logger.WithError(err).Warn("Failed to update backup target")
			}
		}
	}()

	noNeedMonitor := targetSetting.Value == "" || pollInterval == time.Duration(0)
	if noNeedMonitor {
		stopTimer()
		return nil
	}

	if sc.bsTimer != nil {
		if sc.bsTimer.pollInterval == pollInterval {
			// No need to start a new timer if there was one
			return
		}
		// Stop the timer if the poll interval changes
		stopTimer()
	}

	// Start backup store timer
	sc.bsTimer = &BackupStoreTimer{
		logger:       sc.logger.WithField("component", "backup-store-timer"),
		controllerID: sc.controllerID,
		ds:           sc.ds,

		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
	}
	go sc.bsTimer.Start()
	return nil
}

func (sc *SettingController) updateTaintToleration() error {
	setting, err := sc.ds.GetSetting(types.SettingNameTaintToleration)
	if err != nil {
		return err
	}
	newTolerations := setting.Value
	newTolerationsList, err := types.UnmarshalTolerations(newTolerations)
	if err != nil {
		return err
	}
	newTolerationsMap := util.TolerationListToMap(newTolerationsList)

	daemonsetList, err := sc.ds.ListDaemonSetWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn daemonsets for toleration update")
	}

	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn deployments for toleration update")
	}

	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager pods for toleration update")
	}

	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list share manager pods for toleration update")
	}

	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list backing image manager pods for toleration update")
	}

	for _, dp := range deploymentList {
		lastAppliedTolerationsList, err := getLastAppliedTolerationsList(dp)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(util.TolerationListToMap(lastAppliedTolerationsList), newTolerationsMap) {
			continue
		}
		if err := sc.updateTolerationForDeployment(dp, lastAppliedTolerationsList, newTolerationsList); err != nil {
			return err
		}
	}

	for _, ds := range daemonsetList {
		lastAppliedTolerationsList, err := getLastAppliedTolerationsList(ds)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(util.TolerationListToMap(lastAppliedTolerationsList), newTolerationsMap) {
			continue
		}
		if err := sc.updateTolerationForDaemonset(ds, lastAppliedTolerationsList, newTolerationsList); err != nil {
			return err
		}
	}

	pods := append(imPodList, smPodList...)
	pods = append(pods, bimPodList...)
	for _, pod := range pods {
		lastAppliedTolerations, err := getLastAppliedTolerationsList(pod)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(util.TolerationListToMap(lastAppliedTolerations), newTolerationsMap) {
			continue
		}
		sc.logger.Infof("Delete pod %v to update tolerations from %v to %v", pod.Name, util.TolerationListToMap(lastAppliedTolerations), newTolerationsMap)
		if err := sc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SettingController) updateTolerationForDeployment(dp *appsv1.Deployment, lastAppliedTolerations, newTolerations []v1.Toleration) error {
	existingTolerationsMap := util.TolerationListToMap(dp.Spec.Template.Spec.Tolerations)
	lastAppliedTolerationsMap := util.TolerationListToMap(lastAppliedTolerations)
	newTolerationsMap := util.TolerationListToMap(newTolerations)
	dp.Spec.Template.Spec.Tolerations = getFinalTolerations(existingTolerationsMap, lastAppliedTolerationsMap, newTolerationsMap)
	newTolerationsByte, err := json.Marshal(newTolerations)
	if err != nil {
		return err
	}
	if err := util.SetAnnotation(dp, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix), string(newTolerationsByte)); err != nil {
		return err
	}
	sc.logger.Infof("Update tolerations from %v to %v for %v", existingTolerationsMap, dp.Spec.Template.Spec.Tolerations, dp.Name)
	if _, err := sc.ds.UpdateDeployment(dp); err != nil {
		return err
	}
	return nil
}

func (sc *SettingController) updateTolerationForDaemonset(ds *appsv1.DaemonSet, lastAppliedTolerations, newTolerations []v1.Toleration) error {
	existingTolerationsMap := util.TolerationListToMap(ds.Spec.Template.Spec.Tolerations)
	lastAppliedTolerationsMap := util.TolerationListToMap(lastAppliedTolerations)
	newTolerationsMap := util.TolerationListToMap(newTolerations)
	ds.Spec.Template.Spec.Tolerations = getFinalTolerations(existingTolerationsMap, lastAppliedTolerationsMap, newTolerationsMap)
	newTolerationsByte, err := json.Marshal(newTolerations)
	if err != nil {
		return err
	}
	if err := util.SetAnnotation(ds, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix), string(newTolerationsByte)); err != nil {
		return err
	}
	sc.logger.Infof("Update tolerations from %v to %v for %v", existingTolerationsMap, ds.Spec.Template.Spec.Tolerations, ds.Name)
	if _, err := sc.ds.UpdateDaemonSet(ds); err != nil {
		return err
	}
	return nil
}

func getLastAppliedTolerationsList(obj runtime.Object) ([]v1.Toleration, error) {
	lastAppliedTolerations, err := util.GetAnnotation(obj, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix))
	if err != nil {
		return nil, err
	}

	if lastAppliedTolerations == "" {
		lastAppliedTolerations = "[]"
	}

	lastAppliedTolerationsList := []v1.Toleration{}
	if err := json.Unmarshal([]byte(lastAppliedTolerations), &lastAppliedTolerationsList); err != nil {
		return nil, err
	}

	return lastAppliedTolerationsList, nil
}

func (sc *SettingController) updatePriorityClass() error {
	setting, err := sc.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return err
	}
	newPriorityClass := setting.Value

	daemonsetList, err := sc.ds.ListDaemonSetWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn daemonsets for priority class update")
	}

	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn deployments for priority class update")
	}

	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager pods for priority class update")
	}

	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list share manager pods for priority class update")
	}

	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list backing image manager pods for priority class update")
	}

	for _, dp := range deploymentList {
		if dp.Spec.Template.Spec.PriorityClassName == newPriorityClass {
			continue
		}
		sc.logger.Infof("Update the priority class from %v to %v for %v", dp.Spec.Template.Spec.PriorityClassName, newPriorityClass, dp.Name)
		dp.Spec.Template.Spec.PriorityClassName = newPriorityClass
		if _, err := sc.ds.UpdateDeployment(dp); err != nil {
			return err
		}
	}
	for _, ds := range daemonsetList {
		if ds.Spec.Template.Spec.PriorityClassName == newPriorityClass {
			continue
		}
		sc.logger.Infof("Update the priority class from %v to %v for %v", ds.Spec.Template.Spec.PriorityClassName, newPriorityClass, ds.Name)
		ds.Spec.Template.Spec.PriorityClassName = newPriorityClass
		if _, err := sc.ds.UpdateDaemonSet(ds); err != nil {
			return err
		}
	}

	pods := append(imPodList, smPodList...)
	pods = append(pods, bimPodList...)
	for _, pod := range pods {
		if pod.Spec.PriorityClassName == newPriorityClass {
			continue
		}
		sc.logger.Infof("Delete pod %v to update the priority class from %v to %v", pod.Name, pod.Spec.PriorityClassName, newPriorityClass)
		if err := sc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SettingController) updateKubernetesClusterAutoscalerEnabled() error {
	// IM pods annotation will be handled in the instance manager controller

	clusterAutoscalerEnabled, err := sc.ds.GetSettingAsBool(types.SettingNameKubernetesClusterAutoscalerEnabled)
	if err != nil {
		return err
	}

	evictKey := types.KubernetesClusterAutoscalerSafeToEvictKey

	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn deployments for %v annotation update", types.KubernetesClusterAutoscalerSafeToEvictKey)
	}

	longhornUI, err := sc.ds.GetDeployment(types.LonghornUIDeploymentName)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get %v deployment", types.LonghornUIDeploymentName)
	}

	if longhornUI != nil {
		deploymentList = append(deploymentList, longhornUI)
	}

	for _, dp := range deploymentList {
		if !util.HasLocalStorageInDeployment(dp) {
			continue
		}

		anno := dp.Spec.Template.Annotations
		if anno == nil {
			anno = map[string]string{}
		}
		if clusterAutoscalerEnabled {
			if value, exists := anno[evictKey]; exists && value == strconv.FormatBool(clusterAutoscalerEnabled) {
				continue
			}

			anno[evictKey] = strconv.FormatBool(clusterAutoscalerEnabled)
			sc.logger.Infof("Update the %v annotation to %v for %v", types.KubernetesClusterAutoscalerSafeToEvictKey, clusterAutoscalerEnabled, dp.Name)
		} else {
			if _, exists := anno[evictKey]; !exists {
				continue
			}

			delete(anno, evictKey)
			sc.logger.Infof("Delete the %v annotation for %v", types.KubernetesClusterAutoscalerSafeToEvictKey, clusterAutoscalerEnabled, dp.Name)
		}
		dp.Spec.Template.Annotations = anno
		if _, err := sc.ds.UpdateDeployment(dp); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SettingController) updateCNI() error {
	storageNetwork, err := sc.ds.GetSetting(types.SettingNameStorageNetwork)
	if err != nil {
		return err
	}

	volumesDetached, err := sc.ds.AreAllVolumesDetached()
	if err != nil {
		return errors.Wrapf(err, "failed to check volume detachment for %v setting update", types.SettingNameStorageNetwork)
	}

	if !volumesDetached {
		return errors.Errorf("cannot apply %v setting to Longhorn workloads when there are attached volumes", types.SettingNameStorageNetwork)
	}

	nadAnnot := string(types.CNIAnnotationNetworks)
	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager Pods for %v setting update", types.SettingNameStorageNetwork)
	}

	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list backing image manager Pods for %v setting update", types.SettingNameStorageNetwork)
	}

	pods := append(imPodList, bimPodList...)
	for _, pod := range pods {
		if pod.Annotations[nadAnnot] == storageNetwork.Value {
			continue
		}

		if err := sc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
	}

	return nil
}

func getFinalTolerations(existingTolerations, lastAppliedTolerations, newTolerations map[string]v1.Toleration) []v1.Toleration {
	resultMap := make(map[string]v1.Toleration)

	for k, v := range existingTolerations {
		resultMap[k] = v
	}

	for k := range lastAppliedTolerations {
		delete(resultMap, k)
	}

	for k, v := range newTolerations {
		resultMap[k] = v
	}

	resultSlice := []v1.Toleration{}
	for _, v := range resultMap {
		resultSlice = append(resultSlice, v)
	}

	return resultSlice
}

func (sc *SettingController) updateNodeSelector() error {
	setting, err := sc.ds.GetSetting(types.SettingNameSystemManagedComponentsNodeSelector)
	if err != nil {
		return err
	}
	newNodeSelector, err := types.UnmarshalNodeSelector(setting.Value)
	if err != nil {
		return err
	}
	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn deployments for node selector update")
	}
	daemonsetList, err := sc.ds.ListDaemonSetWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn daemonsets for node selector update")
	}
	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager pods for node selector update")
	}
	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list share manager pods for node selector update")
	}
	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list backing image manager pods for node selector update")
	}
	for _, dp := range deploymentList {
		if dp.Spec.Template.Spec.NodeSelector == nil {
			if len(newNodeSelector) == 0 {
				continue
			}
		}
		if reflect.DeepEqual(dp.Spec.Template.Spec.NodeSelector, newNodeSelector) {
			continue
		}
		sc.logger.Infof("Update the node selector from %v to %v for %v", dp.Spec.Template.Spec.NodeSelector, newNodeSelector, dp.Name)
		dp.Spec.Template.Spec.NodeSelector = newNodeSelector
		if _, err := sc.ds.UpdateDeployment(dp); err != nil {
			return err
		}
	}
	for _, ds := range daemonsetList {
		if ds.Spec.Template.Spec.NodeSelector == nil {
			if len(newNodeSelector) == 0 {
				continue
			}
		}
		if reflect.DeepEqual(ds.Spec.Template.Spec.NodeSelector, newNodeSelector) {
			continue
		}
		sc.logger.Infof("Update the node selector from %v to %v for %v", ds.Spec.Template.Spec.NodeSelector, newNodeSelector, ds.Name)
		ds.Spec.Template.Spec.NodeSelector = newNodeSelector
		if _, err := sc.ds.UpdateDaemonSet(ds); err != nil {
			return err
		}
	}
	pods := append(imPodList, smPodList...)
	pods = append(pods, bimPodList...)
	for _, pod := range pods {
		if pod.Spec.NodeSelector == nil {
			if len(newNodeSelector) == 0 {
				continue
			}
		}
		if reflect.DeepEqual(pod.Spec.NodeSelector, newNodeSelector) {
			continue
		}
		if pod.DeletionTimestamp == nil {
			sc.logger.Infof("Delete pod %v to update the node selector from %v to %v", pod.Name, pod.Spec.NodeSelector, newNodeSelector)
			if err := sc.ds.DeletePod(pod.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bst *BackupStoreTimer) Start() {
	if bst == nil {
		return
	}
	log := bst.logger.WithFields(logrus.Fields{
		"interval": bst.pollInterval,
	})
	log.Debug("Start backup store timer")

	wait.PollUntil(bst.pollInterval, func() (done bool, err error) {
		backupTarget, err := bst.ds.GetBackupTarget(types.DefaultBackupTargetName)
		if err != nil {
			log.WithError(err).Errorf("Cannot get %s backup target", types.DefaultBackupTargetName)
			return false, err
		}

		backupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
		if _, err = bst.ds.UpdateBackupTarget(backupTarget); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Warn("Failed to updating backup target")
		}
		log.Debug("Trigger sync backup target")
		return false, nil
	}, bst.stopCh)

	log.Debug("Stop backup store timer")
}

func (bst *BackupStoreTimer) Stop() {
	if bst == nil {
		return
	}
	bst.stopCh <- struct{}{}
}

func (sc *SettingController) syncUpgradeChecker() error {
	upgradeCheckerEnabled, err := sc.ds.GetSettingAsBool(types.SettingNameUpgradeChecker)
	if err != nil {
		return err
	}

	latestLonghornVersion, err := sc.ds.GetSetting(types.SettingNameLatestLonghornVersion)
	if err != nil {
		return err
	}
	stableLonghornVersions, err := sc.ds.GetSetting(types.SettingNameStableLonghornVersions)
	if err != nil {
		return err
	}

	if !upgradeCheckerEnabled {
		if latestLonghornVersion.Value != "" {
			latestLonghornVersion.Value = ""
			if _, err := sc.ds.UpdateSetting(latestLonghornVersion); err != nil {
				return err
			}
		}
		if stableLonghornVersions.Value != "" {
			stableLonghornVersions.Value = ""
			if _, err := sc.ds.UpdateSetting(stableLonghornVersions); err != nil {
				return err
			}
		}
		// reset timestamp so it can be triggered immediately when
		// setting changes next time
		sc.lastUpgradeCheckedTimestamp = time.Time{}
		return nil
	}

	now := time.Now()
	if now.Before(sc.lastUpgradeCheckedTimestamp.Add(upgradeCheckInterval)) {
		return nil
	}

	currentLatestVersion := latestLonghornVersion.Value
	currentStableVersions := stableLonghornVersions.Value
	latestLonghornVersion.Value, stableLonghornVersions.Value, err = sc.CheckLatestAndStableLonghornVersions()
	if err != nil {
		// non-critical error, don't retry
		sc.logger.WithError(err).Debug("Failed to check for the latest and stable Longhorn versions")
		return nil
	}

	sc.lastUpgradeCheckedTimestamp = now

	if latestLonghornVersion.Value != currentLatestVersion {
		sc.logger.Infof("Latest Longhorn version is %v", latestLonghornVersion.Value)
		if _, err := sc.ds.UpdateSetting(latestLonghornVersion); err != nil {
			// non-critical error, don't retry
			sc.logger.WithError(err).Debug("Cannot update latest Longhorn version")
			return nil
		}
	}
	if stableLonghornVersions.Value != currentStableVersions {
		sc.logger.Infof("The latest stable version of every minor release line: %v", stableLonghornVersions.Value)
		if _, err := sc.ds.UpdateSetting(stableLonghornVersions); err != nil {
			// non-critical error, don't retry
			sc.logger.WithError(err).Debug("Cannot update stable Longhorn versions")
			return nil
		}
	}

	return nil
}

func (sc *SettingController) CheckLatestAndStableLonghornVersions() (string, string, error) {
	var (
		resp    CheckUpgradeResponse
		content bytes.Buffer
	)
	kubeVersion, err := sc.kubeClient.Discovery().ServerVersion()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get Kubernetes server version")
	}

	req := &CheckUpgradeRequest{
		AppVersion: sc.version,
		ExtraInfo:  map[string]string{"kubernetesVersion": kubeVersion.GitVersion},
	}
	if err := json.NewEncoder(&content).Encode(req); err != nil {
		return "", "", err
	}
	r, err := http.Post(checkUpgradeURL, "application/json", &content)
	if err != nil {
		return "", "", err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		message := ""
		messageBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			message = err.Error()
		} else {
			message = string(messageBytes)
		}
		return "", "", fmt.Errorf("query return status code %v, message %v", r.StatusCode, message)
	}
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return "", "", err
	}

	latestVersion := ""
	stableVersions := []string{}
	for _, v := range resp.Versions {
		for _, tag := range v.Tags {
			if tag == VersionTagLatest {
				latestVersion = v.Name
			}
			if tag == VersionTagStable {
				stableVersions = append(stableVersions, v.Name)
			}
		}
	}
	if latestVersion == "" {
		return "", "", fmt.Errorf("cannot find latest Longhorn version during CheckLatestAndStableLonghornVersions")
	}
	sort.Strings(stableVersions)
	return latestVersion, strings.Join(stableVersions, ","), nil
}

func (sc *SettingController) enqueueSetting(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	sc.queue.Add(key)
}

func (sc *SettingController) enqueueSettingForNode(obj interface{}) {
	if _, ok := obj.(*longhorn.Node); !ok {
		// Ignore deleted node
		return
	}

	sc.queue.Add(sc.namespace + "/" + string(types.SettingNameGuaranteedEngineManagerCPU))
	sc.queue.Add(sc.namespace + "/" + string(types.SettingNameGuaranteedReplicaManagerCPU))
	sc.queue.Add(sc.namespace + "/" + string(types.SettingNameBackupTarget))
}

func (sc *SettingController) enqueueSettingForBackupTarget(obj interface{}) {
	if _, ok := obj.(*longhorn.BackupTarget); !ok {
		return
	}
	sc.queue.Add(sc.namespace + "/" + string(types.SettingNameBackupTarget))
}

func (sc *SettingController) updateInstanceManagerCPURequest() error {
	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager pods for toleration update")
	}
	imMap, err := sc.ds.ListInstanceManagers()
	if err != nil {
		return err
	}
	for _, imPod := range imPodList {
		if _, exists := imMap[imPod.Name]; !exists {
			continue
		}
		lhNode, err := sc.ds.GetNode(imPod.Spec.NodeName)
		if err != nil {
			return err
		}
		if types.GetCondition(lhNode.Status.Conditions, longhorn.NodeConditionTypeReady).Status != longhorn.ConditionStatusTrue {
			continue
		}

		resourceReq, err := GetInstanceManagerCPURequirement(sc.ds, imPod.Name)
		if err != nil {
			return err
		}
		podResourceReq := imPod.Spec.Containers[0].Resources
		if IsSameGuaranteedCPURequirement(resourceReq, &podResourceReq) {
			continue
		}
		sc.logger.Infof("Delete instance manager pod %v to refresh CPU request option", imPod.Name)
		if err := sc.ds.DeletePod(imPod.Name); err != nil {
			return err
		}
	}

	return nil
}

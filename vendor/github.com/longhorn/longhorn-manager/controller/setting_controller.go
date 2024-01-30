package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/constant"
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
	metricsClient metricsclientset.Interface
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
	AppVersion string `json:"appVersion"`

	ExtraTagInfo   CheckUpgradeExtraInfo `json:"extraTagInfo"`
	ExtraFieldInfo CheckUpgradeExtraInfo `json:"extraFieldInfo"`
}

type CheckUpgradeExtraInfo interface {
}

type CheckUpgradeResponse struct {
	Versions []Version `json:"versions"`
}

func NewSettingController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	metricsClient metricsclientset.Interface,
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
		metricsClient: metricsClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-setting-controller"}),

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

	sc.logger.Info("Starting Longhorn Setting controller")
	defer sc.logger.Info("Shut down Longhorn Setting controller")

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

	log := sc.logger.WithField("Setting", key)
	if sc.queue.NumRequeues(key) < maxRetries {
		handleReconcileErrorLogging(log, err, "Failed to sync Longhorn setting")
		sc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	handleReconcileErrorLogging(log, err, "Dropping Longhorn setting out of the queue")
	sc.queue.Forget(key)
}

func (sc *SettingController) syncSetting(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync setting for %v", key)
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
	case string(types.SettingNameGuaranteedInstanceManagerCPU):
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
	case string(types.SettingNameSupportBundleFailedHistoryLimit):
		if err := sc.cleanupFailedSupportBundles(); err != nil {
			return err
		}
	case string(types.SettingNameLogLevel):
		if err := sc.updateLogLevel(); err != nil {
			return err
		}
	case string(types.SettingNameV2DataEngine):
		if err := sc.updateV2DataEngine(); err != nil {
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
		err = errors.Wrap(err, "failed to sync backup target")
	}()

	stopTimer := func() {
		if sc.bsTimer != nil {
			sc.bsTimer.Stop()
			sc.bsTimer = nil
		}
	}

	responsibleNodeID, err := getResponsibleNodeID(sc.ds)
	if err != nil {
		return errors.Wrap(err, "failed to select node for sync backup target")
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
			return errors.Wrap(err, "failed to create backup target")
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
			if err = sc.handleSecretsForAWSIAMRoleAnnotation(backupTarget.Spec.BackupTargetURL, existingBackupTarget.Spec.CredentialSecret, secretSetting.Value, existingBackupTarget.Spec.BackupTargetURL != targetSetting.Value); err != nil {
				sc.logger.WithError(err).Warn("Failed to update secrets for AWSIAMRoleAnnotation")
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

func (sc *SettingController) handleSecretsForAWSIAMRoleAnnotation(backupTargetURL, oldSecretName, newSecretName string, isBackupTargetURLChanged bool) (err error) {
	isSameSecretName := oldSecretName == newSecretName
	if isSameSecretName && !isBackupTargetURLChanged {
		return nil
	}

	isArnExists := false
	if !isSameSecretName {
		isArnExists, _, err = sc.updateSecretForAWSIAMRoleAnnotation(backupTargetURL, oldSecretName, true)
		if err != nil {
			return err
		}
	}
	_, isValidSecret, err := sc.updateSecretForAWSIAMRoleAnnotation(backupTargetURL, newSecretName, false)
	if err != nil {
		return err
	}
	// kubernetes_secret_controller will not reconcile the secret that does not exist such as named "", so we remove AWS IAM role annotation of pods if new secret name is "".
	if !isValidSecret && isArnExists {
		if err = sc.removePodsAWSIAMRoleAnnotation(); err != nil {
			return err
		}
	}
	return nil
}

// updateSecretForAWSIAMRoleAnnotation adds the AWS IAM Role annotation to make an update to reconcile in kubernetes secret controller and returns
//
//	isArnExists = true if annotation had been added to the secret for first parameter,
//	isValidSecret = true if this secret is valid for second parameter.
//	err != nil if there is an error occurred.
func (sc *SettingController) updateSecretForAWSIAMRoleAnnotation(backupTargetURL, secretName string, isOldSecret bool) (isArnExists bool, isValidSecret bool, err error) {
	if secretName == "" {
		return false, false, nil
	}

	secret, err := sc.ds.GetSecret(sc.namespace, secretName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}

	if isOldSecret {
		delete(secret.Annotations, types.GetLonghornLabelKey(string(types.SettingNameBackupTarget)))
		isArnExists = secret.Data[types.AWSIAMRoleArn] != nil
	} else {
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations[types.GetLonghornLabelKey(string(types.SettingNameBackupTarget))] = backupTargetURL
	}

	if _, err = sc.ds.UpdateSecret(sc.namespace, secret); err != nil {
		return false, false, err
	}
	return isArnExists, true, nil
}

func (sc *SettingController) removePodsAWSIAMRoleAnnotation() error {
	managerPods, err := sc.ds.ListManagerPods()
	if err != nil {
		return err
	}

	instanceManagerPods, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return err
	}
	pods := append(managerPods, instanceManagerPods...)

	for _, pod := range pods {
		_, exist := pod.Annotations[types.AWSIAMRoleAnnotation]
		if !exist {
			continue
		}
		delete(pod.Annotations, types.AWSIAMRoleAnnotation)
		sc.logger.Infof("Removing AWS IAM role for pod %v", pod.Name)
		if _, err := sc.ds.UpdatePod(pod); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}
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
		return errors.Wrap(err, "failed to list Longhorn daemonsets for toleration update")
	}

	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrap(err, "failed to list Longhorn deployments for toleration update")
	}

	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list instance manager pods for toleration update")
	}

	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list share manager pods for toleration update")
	}

	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list backing image manager pods for toleration update")
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
		sc.logger.Infof("Deleting pod %v to update tolerations from %v to %v", pod.Name, util.TolerationListToMap(lastAppliedTolerations), newTolerationsMap)
		if err := sc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SettingController) updateTolerationForDeployment(dp *appsv1.Deployment, lastAppliedTolerations, newTolerations []corev1.Toleration) error {
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
	sc.logger.Infof("Updating tolerations from %v to %v for %v", existingTolerationsMap, dp.Spec.Template.Spec.Tolerations, dp.Name)
	if _, err := sc.ds.UpdateDeployment(dp); err != nil {
		return err
	}
	return nil
}

func (sc *SettingController) updateTolerationForDaemonset(ds *appsv1.DaemonSet, lastAppliedTolerations, newTolerations []corev1.Toleration) error {
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
	sc.logger.Infof("Updating tolerations from %v to %v for %v", existingTolerationsMap, ds.Spec.Template.Spec.Tolerations, ds.Name)
	if _, err := sc.ds.UpdateDaemonSet(ds); err != nil {
		return err
	}
	return nil
}

func getLastAppliedTolerationsList(obj runtime.Object) ([]corev1.Toleration, error) {
	lastAppliedTolerations, err := util.GetAnnotation(obj, types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix))
	if err != nil {
		return nil, err
	}

	if lastAppliedTolerations == "" {
		lastAppliedTolerations = "[]"
	}

	lastAppliedTolerationsList := []corev1.Toleration{}
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
		return errors.Wrap(err, "failed to list Longhorn daemonsets for priority class update")
	}

	deploymentList, err := sc.ds.ListDeploymentWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrap(err, "failed to list Longhorn deployments for priority class update")
	}

	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list instance manager pods for priority class update")
	}

	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list share manager pods for priority class update")
	}

	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list backing image manager pods for priority class update")
	}

	for _, dp := range deploymentList {
		if dp.Spec.Template.Spec.PriorityClassName == newPriorityClass {
			continue
		}
		sc.logger.Infof("Updating the priority class from %v to %v for %v", dp.Spec.Template.Spec.PriorityClassName, newPriorityClass, dp.Name)
		dp.Spec.Template.Spec.PriorityClassName = newPriorityClass
		if _, err := sc.ds.UpdateDeployment(dp); err != nil {
			return err
		}
	}
	for _, ds := range daemonsetList {
		if ds.Spec.Template.Spec.PriorityClassName == newPriorityClass {
			continue
		}
		sc.logger.Infof("Updating the priority class from %v to %v for %v", ds.Spec.Template.Spec.PriorityClassName, newPriorityClass, ds.Name)
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
		sc.logger.Infof("Deleting pod %v to update the priority class from %v to %v", pod.Name, pod.Spec.PriorityClassName, newPriorityClass)
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
			sc.logger.Infof("Updating the %v annotation to %v for %v", types.KubernetesClusterAutoscalerSafeToEvictKey, clusterAutoscalerEnabled, dp.Name)
		} else {
			if _, exists := anno[evictKey]; !exists {
				continue
			}

			delete(anno, evictKey)
			sc.logger.Infof("Deleting the %v annotation for %v", types.KubernetesClusterAutoscalerSafeToEvictKey, clusterAutoscalerEnabled, dp.Name)
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
		return &types.ErrorInvalidState{Reason: fmt.Sprintf("failed to apply %v setting to Longhorn workloads when there are attached volumes", types.SettingNameStorageNetwork)}
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

func (sc *SettingController) updateLogLevel() error {
	setting, err := sc.ds.GetSetting(types.SettingNameLogLevel)
	if err != nil {
		return err
	}
	oldLevel := logrus.GetLevel()
	newLevel, err := logrus.ParseLevel(setting.Value)
	if err != nil {
		return err
	}
	if oldLevel != newLevel {
		logrus.Infof("Updating log level from %v to %v", oldLevel, newLevel)
		logrus.SetLevel(newLevel)
	}

	return nil
}

func (sc *SettingController) updateV2DataEngine() error {
	v2DataEngineEnabled, err := sc.ds.GetSettingAsBool(types.SettingNameV2DataEngine)
	if err != nil {
		return err
	}

	err = sc.ds.ValidateV2DataEngine(v2DataEngineEnabled)
	if err != nil {
		return err
	}

	// Recreate all instance manager pods
	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrapf(err, "failed to list instance manager Pods for %v setting update", types.SettingNameV2DataEngine)
	}

	for _, pod := range imPodList {
		if pod.Annotations[types.V2DataEngineAnnotation] == strconv.FormatBool(v2DataEngineEnabled) {
			continue
		}

		if err := sc.ds.DeletePod(pod.Name); err != nil {
			return err
		}
	}

	return nil
}

func getFinalTolerations(existingTolerations, lastAppliedTolerations, newTolerations map[string]corev1.Toleration) []corev1.Toleration {
	resultMap := make(map[string]corev1.Toleration)

	for k, v := range existingTolerations {
		resultMap[k] = v
	}

	for k := range lastAppliedTolerations {
		delete(resultMap, k)
	}

	for k, v := range newTolerations {
		resultMap[k] = v
	}

	resultSlice := []corev1.Toleration{}
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
		return errors.Wrap(err, "failed to list Longhorn deployments for node selector update")
	}
	daemonsetList, err := sc.ds.ListDaemonSetWithLabels(types.GetBaseLabelsForSystemManagedComponent())
	if err != nil {
		return errors.Wrap(err, "failed to list Longhorn daemonsets for node selector update")
	}
	imPodList, err := sc.ds.ListInstanceManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list instance manager pods for node selector update")
	}
	smPodList, err := sc.ds.ListShareManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list share manager pods for node selector update")
	}
	bimPodList, err := sc.ds.ListBackingImageManagerPods()
	if err != nil {
		return errors.Wrap(err, "failed to list backing image manager pods for node selector update")
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
		sc.logger.Infof("Updating the node selector from %v to %v for %v", dp.Spec.Template.Spec.NodeSelector, newNodeSelector, dp.Name)
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
		sc.logger.Infof("Updating the node selector from %v to %v for %v", ds.Spec.Template.Spec.NodeSelector, newNodeSelector, ds.Name)
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
			sc.logger.Infof("Deleting pod %v to update the node selector from %v to %v", pod.Name, pod.Spec.NodeSelector, newNodeSelector)
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
	log.Info("Starting backup store timer")

	wait.PollUntil(bst.pollInterval, func() (done bool, err error) {
		backupTarget, err := bst.ds.GetBackupTarget(types.DefaultBackupTargetName)
		if err != nil {
			log.WithError(err).Errorf("Failed to get %s backup target", types.DefaultBackupTargetName)
			return false, err
		}

		log.Debug("Triggering sync backup target")
		backupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
		if _, err = bst.ds.UpdateBackupTarget(backupTarget); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Warn("Failed to updating backup target")
		}
		return false, nil
	}, bst.stopCh)

	log.Infof("Stopped backup store timer")
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
		sc.logger.WithError(err).Warn("Failed to check for the latest and stable Longhorn versions")
		return nil
	}

	sc.lastUpgradeCheckedTimestamp = now

	if latestLonghornVersion.Value != currentLatestVersion {
		sc.logger.Infof("Latest Longhorn version is %v", latestLonghornVersion.Value)
		if _, err := sc.ds.UpdateSetting(latestLonghornVersion); err != nil {
			// non-critical error, don't retry
			sc.logger.WithError(err).Warn("Failed to update latest Longhorn version")
			return nil
		}
	}
	if stableLonghornVersions.Value != currentStableVersions {
		sc.logger.Infof("The latest stable version of every minor release line: %v", stableLonghornVersions.Value)
		if _, err := sc.ds.UpdateSetting(stableLonghornVersions); err != nil {
			// non-critical error, don't retry
			sc.logger.WithError(err).Warn("Failed to update stable Longhorn versions")
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

	extraTagInfo, extraFieldInfo, err := sc.GetCheckUpgradeRequestExtraInfo()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get extra info for upgrade checker")
	}
	req := &CheckUpgradeRequest{
		AppVersion:     sc.version,
		ExtraTagInfo:   extraTagInfo,
		ExtraFieldInfo: extraFieldInfo,
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
		messageBytes, err := io.ReadAll(r.Body)
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
		return "", "", fmt.Errorf("failed to find latest Longhorn version during CheckLatestAndStableLonghornVersions")
	}
	sort.Strings(stableVersions)
	return latestVersion, strings.Join(stableVersions, ","), nil
}

func (sc *SettingController) enqueueSetting(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to get key for object %#v: %v", obj, err))
		return
	}

	sc.queue.Add(key)
}

func (sc *SettingController) enqueueSettingForNode(obj interface{}) {
	if _, ok := obj.(*longhorn.Node); !ok {
		// Ignore deleted node
		return
	}

	sc.queue.Add(sc.namespace + "/" + string(types.SettingNameGuaranteedInstanceManagerCPU))
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
		return errors.Wrap(err, "failed to list instance manager pods for toleration update")
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
		sc.logger.Infof("Deleting instance manager pod %v to refresh CPU request option", imPod.Name)
		if err := sc.ds.DeletePod(imPod.Name); err != nil {
			return err
		}
	}

	return nil
}

func (sc *SettingController) cleanupFailedSupportBundles() error {
	failedLimit, err := sc.ds.GetSettingAsInt(types.SettingNameSupportBundleFailedHistoryLimit)
	if err != nil {
		return err
	}

	if failedLimit > 0 {
		return nil
	}

	supportBundleList, err := sc.ds.ListSupportBundles()
	if err != nil {
		return errors.Wrap(err, "failed to list SupportBundles for auto-deletion")
	}

	for _, supportBundle := range supportBundleList {
		if supportBundle.Status.OwnerID != sc.controllerID {
			continue
		}

		if supportBundle.Status.State == longhorn.SupportBundleStatePurging {
			continue
		}
		if supportBundle.Status.State == longhorn.SupportBundleStateDeleting {
			continue
		}
		if supportBundle.Status.State != longhorn.SupportBundleStateError {
			continue
		}

		supportBundle.Status.State = longhorn.SupportBundleStatePurging
		_, err = sc.ds.UpdateSupportBundleStatus(supportBundle)
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to purge SupportBundle %v", supportBundle.Name)
		}

		message := fmt.Sprintf("Purging failed SupportBundle %v", supportBundle.Name)
		sc.logger.Info(message)
		sc.eventRecorder.Eventf(supportBundle, corev1.EventTypeNormal, constant.EventReasonDeleting, message)
	}

	return nil
}

func (sc *SettingController) GetCheckUpgradeRequestExtraInfo() (extraTagInfo CheckUpgradeExtraInfo, extraFieldInfo CheckUpgradeExtraInfo, err error) {
	clusterInfo := &ClusterInfo{
		logger:        sc.logger,
		ds:            sc.ds,
		kubeClient:    sc.kubeClient,
		metricsClient: sc.metricsClient,
		structFields: ClusterInfoStructFields{
			tags:   util.StructFields{},
			fields: util.StructFields{},
		},
		controllerID: sc.controllerID,
		namespace:    sc.namespace,
	}

	defer func() {
		extraTagInfo = clusterInfo.structFields.tags.NewStruct()
		extraFieldInfo = clusterInfo.structFields.fields.NewStruct()
	}()

	kubeVersion, err := sc.kubeClient.Discovery().ServerVersion()
	if err != nil {
		err = errors.Wrap(err, "failed to get Kubernetes server version")
		return
	}
	clusterInfo.structFields.tags.Append(ClusterInfoKubernetesVersion, kubeVersion.GitVersion)

	allowCollectingUsage, err := sc.ds.GetSettingAsBool(types.SettingNameAllowCollectingLonghornUsage)
	if err != nil {
		sc.logger.WithError(err).Warnf("Failed to get Setting %v for extra info collection", types.SettingNameAllowCollectingLonghornUsage)
		return nil, nil, nil
	}

	if !allowCollectingUsage {
		return
	}

	clusterInfo.collectNodeScope()

	responsibleNodeID, err := getResponsibleNodeID(sc.ds)
	if err != nil {
		sc.logger.WithError(err).Warn("Failed to get responsible Node for extra info collection")
		return nil, nil, nil
	}
	if responsibleNodeID == sc.controllerID {
		clusterInfo.collectClusterScope()
	}

	return
}

// Cluster Scope Info: will be sent from one of the Longhorn cluster nodes
const (
	ClusterInfoNamespaceUID = util.StructName("LonghornNamespaceUid")
	ClusterInfoNodeCount    = util.StructName("LonghornNodeCount")

	ClusterInfoVolumeAvgActualSize    = util.StructName("LonghornVolumeAverageActualSizeBytes")
	ClusterInfoVolumeAvgSize          = util.StructName("LonghornVolumeAverageSizeBytes")
	ClusterInfoVolumeAvgSnapshotCount = util.StructName("LonghornVolumeAverageSnapshotCount")
	ClusterInfoVolumeAvgNumOfReplicas = util.StructName("LonghornVolumeAverageNumberOfReplicas")

	ClusterInfoPodAvgCPUUsageFmt                         = "Longhorn%sAverageCpuUsageMilliCores"
	ClusterInfoPodAvgMemoryUsageFmt                      = "Longhorn%sAverageMemoryUsageBytes"
	ClusterInfoSettingFmt                                = "LonghornSetting%s"
	ClusterInfoVolumeAccessModeCountFmt                  = "LonghornVolumeAccessMode%sCount"
	ClusterInfoVolumeDataLocalityCountFmt                = "LonghornVolumeDataLocality%sCount"
	ClusterInfoVolumeFrontendCountFmt                    = "LonghornVolumeFrontend%sCount"
	ClusterInfoVolumeOfflineReplicaRebuildingCountFmt    = "LonghornVolumeOfflineReplicaRebuilding%sCount"
	ClusterInfoVolumeReplicaAutoBalanceCountFmt          = "LonghornVolumeReplicaAutoBalance%sCount"
	ClusterInfoVolumeReplicaSoftAntiAffinityCountFmt     = "LonghornVolumeReplicaSoftAntiAffinity%sCount"
	ClusterInfoVolumeReplicaZoneSoftAntiAffinityCountFmt = "LonghornVolumeReplicaZoneSoftAntiAffinity%sCount"
	ClusterInfoVolumeRestoreVolumeRecurringJobCountFmt   = "LonghornVolumeRestoreVolumeRecurringJob%sCount"
	ClusterInfoVolumeSnapshotDataIntegrityCountFmt       = "LonghornVolumeSnapshotDataIntegrity%sCount"
	ClusterInfoVolumeUnmapMarkSnapChainRemovedCountFmt   = "LonghornVolumeUnmapMarkSnapChainRemoved%sCount"
)

// Node Scope Info: will be sent from all Longhorn cluster nodes
const (
	ClusterInfoKubernetesVersion      = util.StructName("KubernetesVersion")
	ClusterInfoKubernetesNodeProvider = util.StructName("KubernetesNodeProvider")

	ClusterInfoHostKernelRelease = util.StructName("HostKernelRelease")
	ClusterInfoHostOsDistro      = util.StructName("HostOsDistro")

	ClusterInfoNodeDiskCountFmt = "LonghornNodeDisk%sCount"
)

// ClusterInfo struct is used to collect information about the cluster.
// This provides additional usage metrics to https://metrics.longhorn.io.
type ClusterInfo struct {
	logger logrus.FieldLogger

	ds *datastore.DataStore

	kubeClient    clientset.Interface
	metricsClient metricsclientset.Interface

	structFields ClusterInfoStructFields

	controllerID string
	namespace    string

	osDistro string
}

type ClusterInfoStructFields struct {
	tags   util.StructFields
	fields util.StructFields
}

func (info *ClusterInfo) collectClusterScope() {
	if err := info.collectNamespace(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect Longhorn namespace")
	}

	if err := info.collectNodeCount(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect number of Longhorn nodes")
	}

	if err := info.collectResourceUsage(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect Longhorn resource usages")
	}

	if err := info.collectVolumesInfo(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect Longhorn Volumes info")
	}

	if err := info.collectSettings(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect Longhorn settings")
	}
}

func (info *ClusterInfo) collectNamespace() error {
	namespace, err := info.ds.GetNamespace(info.namespace)
	if err == nil {
		info.structFields.fields.Append(ClusterInfoNamespaceUID, string(namespace.UID))
	}
	return err
}

func (info *ClusterInfo) collectNodeCount() error {
	nodesRO, err := info.ds.ListNodesRO()
	if err == nil {
		info.structFields.fields.Append(ClusterInfoNodeCount, len(nodesRO))
	}
	return err
}

func (info *ClusterInfo) collectResourceUsage() error {
	componentMap := map[string]map[string]string{
		"Manager":         info.ds.GetManagerLabel(),
		"InstanceManager": types.GetInstanceManagerComponentLabel(),
	}

	metricsClient := info.metricsClient.MetricsV1beta1()
	for component, label := range componentMap {
		selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
			MatchLabels: label,
		})
		if err != nil {
			logrus.WithError(err).Warnf("Failed to get %v label for %v", label, component)
			continue
		}

		pods, err := info.ds.ListPodsBySelector(selector)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to list %v Pod by %v label", component, label)
			continue
		}
		podCount := len(pods)

		if podCount == 0 {
			continue
		}

		var totalCPUUsage resource.Quantity
		var totalMemoryUsage resource.Quantity
		for _, pod := range pods {
			podMetrics, err := metricsClient.PodMetricses(info.namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				logrus.WithError(err).Warnf("Failed to get %v Pod", pod.Name)
				continue
			}
			for _, container := range podMetrics.Containers {
				totalCPUUsage.Add(*container.Usage.Cpu())
				totalMemoryUsage.Add(*container.Usage.Memory())
			}
		}

		avgCPUUsageMilli := totalCPUUsage.MilliValue() / int64(podCount)
		cpuStruct := util.StructName(fmt.Sprintf(ClusterInfoPodAvgCPUUsageFmt, component))
		info.structFields.fields.Append(cpuStruct, avgCPUUsageMilli)

		avgMemoryUsageBytes := totalMemoryUsage.Value() / int64(podCount)
		memStruct := util.StructName(fmt.Sprintf(ClusterInfoPodAvgMemoryUsageFmt, component))
		info.structFields.fields.Append(memStruct, avgMemoryUsageBytes)
	}

	return nil
}

func (info *ClusterInfo) collectSettings() error {
	includeAsBoolean := map[types.SettingName]bool{
		types.SettingNameTaintToleration:                     true,
		types.SettingNameSystemManagedComponentsNodeSelector: true,
		types.SettingNameRegistrySecret:                      true,
		types.SettingNamePriorityClass:                       true,
		types.SettingNameSnapshotDataIntegrityCronJob:        true,
		types.SettingNameStorageNetwork:                      true,
	}

	include := map[types.SettingName]bool{
		types.SettingNameAllowRecurringJobWhileVolumeDetached:                     true,
		types.SettingNameAllowVolumeCreationWithDegradedAvailability:              true,
		types.SettingNameAutoCleanupSystemGeneratedSnapshot:                       true,
		types.SettingNameAutoDeletePodWhenVolumeDetachedUnexpectedly:              true,
		types.SettingNameAutoSalvage:                                              true,
		types.SettingNameBackingImageCleanupWaitInterval:                          true,
		types.SettingNameBackingImageRecoveryWaitInterval:                         true,
		types.SettingNameBackupCompressionMethod:                                  true,
		types.SettingNameBackupstorePollInterval:                                  true,
		types.SettingNameBackupConcurrentLimit:                                    true,
		types.SettingNameConcurrentAutomaticEngineUpgradePerNodeLimit:             true,
		types.SettingNameConcurrentBackupRestorePerNodeLimit:                      true,
		types.SettingNameConcurrentReplicaRebuildPerNodeLimit:                     true,
		types.SettingNameCRDAPIVersion:                                            true,
		types.SettingNameCreateDefaultDiskLabeledNodes:                            true,
		types.SettingNameDefaultDataLocality:                                      true,
		types.SettingNameDefaultReplicaCount:                                      true,
		types.SettingNameDisableRevisionCounter:                                   true,
		types.SettingNameDisableSchedulingOnCordonedNode:                          true,
		types.SettingNameEngineReplicaTimeout:                                     true,
		types.SettingNameFailedBackupTTL:                                          true,
		types.SettingNameFastReplicaRebuildEnabled:                                true,
		types.SettingNameGuaranteedInstanceManagerCPU:                             true,
		types.SettingNameKubernetesClusterAutoscalerEnabled:                       true,
		types.SettingNameNodeDownPodDeletionPolicy:                                true,
		types.SettingNameNodeDrainPolicy:                                          true,
		types.SettingNameOrphanAutoDeletion:                                       true,
		types.SettingNameRecurringFailedJobsHistoryLimit:                          true,
		types.SettingNameRecurringSuccessfulJobsHistoryLimit:                      true,
		types.SettingNameRemoveSnapshotsDuringFilesystemTrim:                      true,
		types.SettingNameReplicaAutoBalance:                                       true,
		types.SettingNameReplicaFileSyncHTTPClientTimeout:                         true,
		types.SettingNameReplicaReplenishmentWaitInterval:                         true,
		types.SettingNameReplicaSoftAntiAffinity:                                  true,
		types.SettingNameReplicaZoneSoftAntiAffinity:                              true,
		types.SettingNameRestoreConcurrentLimit:                                   true,
		types.SettingNameRestoreVolumeRecurringJobs:                               true,
		types.SettingNameSnapshotDataIntegrity:                                    true,
		types.SettingNameSnapshotDataIntegrityImmediateCheckAfterSnapshotCreation: true,
		types.SettingNameStorageMinimalAvailablePercentage:                        true,
		types.SettingNameStorageOverProvisioningPercentage:                        true,
		types.SettingNameStorageReservedPercentageForDefaultDisk:                  true,
		types.SettingNameSupportBundleFailedHistoryLimit:                          true,
		types.SettingNameSystemManagedPodsImagePullPolicy:                         true,
		types.SettingNameV2DataEngine:                                             true,
		types.SettingNameOfflineReplicaRebuilding:                                 true,
	}

	settings, err := info.ds.ListSettings()
	if err != nil {
		return err
	}

	settingMap := make(map[string]interface{})
	for _, setting := range settings {
		settingName := types.SettingName(setting.Name)

		switch {
		// Setting that require extra processing to identify their general purpose
		case settingName == types.SettingNameBackupTarget:
			settingMap[setting.Name] = types.GetBackupTargetSchemeFromURL(setting.Value)

		// Setting that should be collected as boolean (true if configured, false if not)
		case includeAsBoolean[settingName]:
			definition, ok := types.GetSettingDefinition(types.SettingName(setting.Name))
			if !ok {
				logrus.WithError(err).Warnf("Failed to get Setting %v definition", setting.Name)
				continue
			}
			settingMap[setting.Name] = setting.Value != definition.Default

		// Setting value
		case include[settingName]:
			convertedValue, err := info.convertSettingValueType(setting)
			if err != nil {
				logrus.WithError(err).Warnf("Failed to convert Setting %v value", setting.Name)
				continue
			}
			settingMap[setting.Name] = convertedValue
		}

		if value, ok := settingMap[setting.Name]; ok && value == "" {
			settingMap[setting.Name] = types.ValueEmpty
		}
	}

	for name, value := range settingMap {
		structName := util.StructName(fmt.Sprintf(ClusterInfoSettingFmt, util.ConvertToCamel(name, "-")))

		switch reflect.TypeOf(value).Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
			info.structFields.fields.Append(structName, value)
		default:
			info.structFields.tags.Append(structName, fmt.Sprint(value))
		}
	}
	return nil
}

func (info *ClusterInfo) convertSettingValueType(setting *longhorn.Setting) (convertedValue interface{}, err error) {
	definition, ok := types.GetSettingDefinition(types.SettingName(setting.Name))
	if !ok {
		return false, fmt.Errorf("failed to get Setting %v definition", setting.Name)
	}

	switch definition.Type {
	case types.SettingTypeInt:
		return strconv.ParseInt(setting.Value, 10, 64)
	case types.SettingTypeBool:
		return strconv.ParseBool(setting.Value)
	default:
		return setting.Value, nil
	}
}

func (info *ClusterInfo) collectVolumesInfo() error {
	volumesRO, err := info.ds.ListVolumesRO()
	if err != nil {
		return errors.Wrapf(err, "failed to list Longhorn Volumes")
	}
	volumeCount := len(volumesRO)

	isV2DataEngineEnabled, err := info.ds.GetSettingAsBool(types.SettingNameV2DataEngine)
	if err != nil {
		return err
	}

	var totalVolumeSize int
	var totalVolumeActualSize int
	var totalVolumeNumOfReplicas int
	newStruct := func() map[util.StructName]int { return make(map[util.StructName]int, volumeCount) }
	accessModeCountStruct := newStruct()
	dataLocalityCountStruct := newStruct()
	frontendCountStruct := newStruct()
	offlineReplicaRebuildingCountStruct := newStruct()
	replicaAutoBalanceCountStruct := newStruct()
	replicaSoftAntiAffinityCountStruct := newStruct()
	replicaZoneSoftAntiAffinityCountStruct := newStruct()
	restoreVolumeRecurringJobCountStruct := newStruct()
	snapshotDataIntegrityCountStruct := newStruct()
	unmapMarkSnapChainRemovedCountStruct := newStruct()
	for _, volume := range volumesRO {
		isVolumeV2DataEngineEnabled := isV2DataEngineEnabled && volume.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV2
		if !isVolumeV2DataEngineEnabled {
			totalVolumeSize += int(volume.Spec.Size)
			totalVolumeActualSize += int(volume.Status.ActualSize)
		}
		totalVolumeNumOfReplicas += volume.Spec.NumberOfReplicas

		accessMode := types.ValueUnknown
		if volume.Spec.AccessMode != "" {
			accessMode = util.ConvertToCamel(string(volume.Spec.AccessMode), "-")
		}
		accessModeCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeAccessModeCountFmt, accessMode))]++

		dataLocality := types.ValueUnknown
		if volume.Spec.DataLocality != "" {
			dataLocality = util.ConvertToCamel(string(volume.Spec.DataLocality), "-")
		}
		dataLocalityCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeDataLocalityCountFmt, dataLocality))]++

		if volume.Spec.Frontend != "" && !isVolumeV2DataEngineEnabled {
			frontend := util.ConvertToCamel(string(volume.Spec.Frontend), "-")
			frontendCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeFrontendCountFmt, frontend))]++
		}

		offlineReplicaRebuilding := info.collectSettingInVolume(string(volume.Spec.OfflineReplicaRebuilding), string(longhorn.OfflineReplicaRebuildingIgnored), types.SettingNameOfflineReplicaRebuilding)
		offlineReplicaRebuildingCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeOfflineReplicaRebuildingCountFmt, util.ConvertToCamel(string(offlineReplicaRebuilding), "-")))]++

		replicaAutoBalance := info.collectSettingInVolume(string(volume.Spec.ReplicaAutoBalance), string(longhorn.ReplicaAutoBalanceIgnored), types.SettingNameReplicaAutoBalance)
		replicaAutoBalanceCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeReplicaAutoBalanceCountFmt, util.ConvertToCamel(string(replicaAutoBalance), "-")))]++

		replicaSoftAntiAffinity := info.collectSettingInVolume(string(volume.Spec.ReplicaSoftAntiAffinity), string(longhorn.ReplicaSoftAntiAffinityDefault), types.SettingNameReplicaSoftAntiAffinity)
		replicaSoftAntiAffinityCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeReplicaSoftAntiAffinityCountFmt, util.ConvertToCamel(string(replicaSoftAntiAffinity), "-")))]++

		replicaZoneSoftAntiAffinity := info.collectSettingInVolume(string(volume.Spec.ReplicaZoneSoftAntiAffinity), string(longhorn.ReplicaZoneSoftAntiAffinityDefault), types.SettingNameReplicaZoneSoftAntiAffinity)
		replicaZoneSoftAntiAffinityCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeReplicaZoneSoftAntiAffinityCountFmt, util.ConvertToCamel(string(replicaZoneSoftAntiAffinity), "-")))]++

		restoreVolumeRecurringJob := info.collectSettingInVolume(string(volume.Spec.RestoreVolumeRecurringJob), string(longhorn.RestoreVolumeRecurringJobDefault), types.SettingNameRestoreVolumeRecurringJobs)
		restoreVolumeRecurringJobCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeRestoreVolumeRecurringJobCountFmt, util.ConvertToCamel(string(restoreVolumeRecurringJob), "-")))]++

		snapshotDataIntegrity := info.collectSettingInVolume(string(volume.Spec.SnapshotDataIntegrity), string(longhorn.SnapshotDataIntegrityIgnored), types.SettingNameSnapshotDataIntegrity)
		snapshotDataIntegrityCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeSnapshotDataIntegrityCountFmt, util.ConvertToCamel(string(snapshotDataIntegrity), "-")))]++

		unmapMarkSnapChainRemoved := info.collectSettingInVolume(string(volume.Spec.UnmapMarkSnapChainRemoved), string(longhorn.UnmapMarkSnapChainRemovedIgnored), types.SettingNameRemoveSnapshotsDuringFilesystemTrim)
		unmapMarkSnapChainRemovedCountStruct[util.StructName(fmt.Sprintf(ClusterInfoVolumeUnmapMarkSnapChainRemovedCountFmt, util.ConvertToCamel(string(unmapMarkSnapChainRemoved), "-")))]++
	}
	info.structFields.fields.AppendCounted(accessModeCountStruct)
	info.structFields.fields.AppendCounted(dataLocalityCountStruct)
	info.structFields.fields.AppendCounted(frontendCountStruct)
	info.structFields.fields.AppendCounted(offlineReplicaRebuildingCountStruct)
	info.structFields.fields.AppendCounted(replicaAutoBalanceCountStruct)
	info.structFields.fields.AppendCounted(replicaSoftAntiAffinityCountStruct)
	info.structFields.fields.AppendCounted(replicaZoneSoftAntiAffinityCountStruct)
	info.structFields.fields.AppendCounted(restoreVolumeRecurringJobCountStruct)
	info.structFields.fields.AppendCounted(snapshotDataIntegrityCountStruct)
	info.structFields.fields.AppendCounted(unmapMarkSnapChainRemovedCountStruct)

	var avgVolumeSnapshotCount int
	var avgVolumeSize int
	var avgVolumeActualSize int
	var avgVolumeNumOfReplicas int
	if volumeCount > 0 {
		if totalVolumeSize > 0 {
			avgVolumeSize = totalVolumeSize / volumeCount

			if totalVolumeActualSize > 0 {
				avgVolumeActualSize = totalVolumeActualSize / volumeCount
			}
		}

		avgVolumeNumOfReplicas = totalVolumeNumOfReplicas / volumeCount

		snapshotsRO, err := info.ds.ListSnapshotsRO(labels.Everything())
		if err != nil {
			return errors.Wrapf(err, "failed to list Longhorn Snapshots")
		}
		avgVolumeSnapshotCount = len(snapshotsRO) / volumeCount
	}
	info.structFields.fields.Append(ClusterInfoVolumeAvgSize, avgVolumeSize)
	info.structFields.fields.Append(ClusterInfoVolumeAvgActualSize, avgVolumeActualSize)
	info.structFields.fields.Append(ClusterInfoVolumeAvgSnapshotCount, avgVolumeSnapshotCount)
	info.structFields.fields.Append(ClusterInfoVolumeAvgNumOfReplicas, avgVolumeNumOfReplicas)

	return nil
}

func (info *ClusterInfo) collectSettingInVolume(volumeSpecValue, ignoredValue string, settingName types.SettingName) string {
	if volumeSpecValue == ignoredValue {
		globalSetting, err := info.ds.GetSetting(settingName)
		if err != nil {
			info.logger.WithError(err).Warnf("Failed to get Longhorn Setting %v", settingName)
		}
		return globalSetting.Value
	}
	return volumeSpecValue
}

func (info *ClusterInfo) collectNodeScope() {
	if err := info.collectHostKernelRelease(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect host kernel release")
	}

	if err := info.collectHostOSDistro(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect host OS distro")
	}

	if err := info.collectNodeDiskCount(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect number of node disks")
	}

	if err := info.collectKubernetesNodeProvider(); err != nil {
		info.logger.WithError(err).Warn("Failed to collect node provider")
	}
}

func (info *ClusterInfo) collectHostKernelRelease() error {
	kernelRelease, err := util.GetHostKernelRelease()
	if err == nil {
		info.structFields.tags.Append(ClusterInfoHostKernelRelease, kernelRelease)
	}
	return err
}

func (info *ClusterInfo) collectHostOSDistro() (err error) {
	if info.osDistro == "" {
		info.osDistro, err = util.GetHostOSDistro()
		if err != nil {
			return err
		}
	}
	info.structFields.tags.Append(ClusterInfoHostOsDistro, info.osDistro)
	return nil
}

func (info *ClusterInfo) collectKubernetesNodeProvider() error {
	node, err := info.ds.GetKubernetesNode(info.controllerID)
	if err == nil {
		scheme := types.GetKubernetesProviderNameFromURL(node.Spec.ProviderID)
		info.structFields.tags.Append(ClusterInfoKubernetesNodeProvider, scheme)
	}
	return err
}

func (info *ClusterInfo) collectNodeDiskCount() error {
	node, err := info.ds.GetNodeRO(info.controllerID)
	if err != nil {
		return err
	}

	structMap := make(map[util.StructName]int)
	for _, disk := range node.Spec.Disks {
		deviceType, err := types.GetDeviceTypeOf(disk.Path)
		if err != nil {
			info.logger.WithError(err).Warnf("Failed to get device type of %v", disk.Path)
			deviceType = types.ValueUnknown
		}
		structMap[util.StructName(fmt.Sprintf(ClusterInfoNodeDiskCountFmt, strings.ToUpper(deviceType)))]++
	}
	for structName, value := range structMap {
		info.structFields.fields.Append(structName, value)
	}

	return nil
}

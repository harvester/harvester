package controller

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/longhorn/backupstore"

	systembackupstore "github.com/longhorn/backupstore/systembackup"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SystemRolloutControllerName = "longhorn-system-rollout"
	SystemRolloutNamePrefix     = "longhorn-system-rollout-"

	SystemRolloutErrFailedConvertToObjectFmt = "failed converting %v to %v object"
	SystemRolloutErrFailedToCreateFmt        = "failed to create item: %v %v"
	SystemRolloutErrMissingDependencyFmt     = "cannot rollout %v due to missing dependency: %v %v"

	SystemRolloutMsgDownloadedFmt       = "Downloaded from %v"
	SystemRolloutMsgIdentical           = "identical"
	SystemRolloutMsgInitializedFmt      = "Initialized system rollout for %v"
	SystemRolloutMsgRestoringFmt        = "Restoring %v"
	SystemRolloutMsgRequeueNextPhaseFmt = "Requeue for next phase: %v"
	SystemRolloutMsgRequeueDueToFmt     = "Requeue due to %v"
	SystemRolloutMsgUnpackedFmt         = "Unpacked %v"

	SystemRolloutMsgCompleted       = "System rollout completed"
	SystemRolloutMsgCreating        = "System rollout creating"
	SystemRolloutMsgIgnoreItemFmt   = "System rollout ignoring item: %v"
	SystemRolloutMsgRestoredItem    = "System rollout restored item"
	SystemRolloutMsgRestoredKindFmt = "System rollout restored Kind: %v"
	SystemRolloutMsgUpdating        = "System rollout updating"
)

var (
	SystemRolloutMsgSkipIdentical = fmt.Sprintf(SystemRolloutMsgIgnoreItemFmt, SystemRolloutMsgIdentical)
)

type systemRolloutRecordType string

const (
	systemRolloutRecordTypeError  = systemRolloutRecordType("error")
	systemRolloutRecordTypeNone   = systemRolloutRecordType("")
	systemRolloutRecordTypeNormal = systemRolloutRecordType("normal")
)

type systemRolloutRecord struct {
	nextState longhorn.SystemRestoreState

	recordType systemRolloutRecordType
	message    string
	reason     string
}

type extractedResources struct {
	customResourceDefinitionList *apiextensionsv1.CustomResourceDefinitionList

	clusterRoleList        *rbacv1.ClusterRoleList
	clusterRoleBindingList *rbacv1.ClusterRoleBindingList
	roleList               *rbacv1.RoleList
	roleBindingList        *rbacv1.RoleBindingList

	daemonSetList  *appsv1.DaemonSetList
	deploymentList *appsv1.DeploymentList

	configMapList             *corev1.ConfigMapList
	persistentVolumeList      *corev1.PersistentVolumeList
	persistentVolumeClaimList *corev1.PersistentVolumeClaimList
	serviceList               *corev1.ServiceList
	serviceAccountList        *corev1.ServiceAccountList

	storageClassList *storagev1.StorageClassList

	podSecurityPolicyList *policyv1beta1.PodSecurityPolicyList

	engineImageList  *longhorn.EngineImageList
	recurringJobList *longhorn.RecurringJobList
	settingList      *longhorn.SettingList
	volumeList       *longhorn.VolumeList
}

type SystemRolloutController struct {
	*baseController

	controllerID string
	stopCh       chan struct{}

	kubeClient clientset.Interface

	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	backupTargetClient     engineapi.SystemBackupOperationInterface
	backupTargetURL        string
	backupTargetCredential map[string]string

	systemRestore        *longhorn.SystemRestore
	systemRestoreName    string
	systemRestoreVersion string

	systemRestoredAt  string
	systemRestoredURL string

	downloadPath string
	engineImage  string

	extractedResources

	cacheErrors util.MultiError
	cacheSyncs  []cache.InformerSynced
}

func NewSystemRolloutController(
	systemRestoreName string,
	logger logrus.FieldLogger,
	controllerID string,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	stopCh chan struct{},
	kubeClient clientset.Interface,
	extensionsClient apiextensionsclientset.Interface,
) *SystemRolloutController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	c := &SystemRolloutController{
		baseController: newBaseControllerWithQueue(SystemRolloutControllerName, logger,
			workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(100*time.Millisecond, 2*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(100), 1000)},
			), SystemRolloutControllerName),
		),
		controllerID: controllerID,
		stopCh:       stopCh,

		kubeClient: kubeClient,

		ds:            ds,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: SystemRolloutControllerName + "-controller"}),

		systemRestoreName: systemRestoreName,
	}

	ds.SystemRestoreInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.enqueue() },
		UpdateFunc: func(old, cur interface{}) { c.enqueue() },
	})
	c.cacheSyncs = append(c.cacheSyncs, ds.SystemRestoreInformer.HasSynced)

	return c
}

func (c *SystemRolloutController) enqueue() {
	c.queue.Add("system-rollout")
}

func (c *SystemRolloutController) Run() error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	if !cache.WaitForNamedCacheSync("longhorn system rollout", c.stopCh, c.cacheSyncs...) {
		return fmt.Errorf("failed to sync informers")
	}

	startTime := time.Now()
	defer func() {
		log := c.logger.WithField("runtime", time.Since(startTime))
		log.Info("Closing")
	}()
	go wait.Until(c.worker, time.Second, c.stopCh)

	<-c.stopCh
	return nil
}

func (c *SystemRolloutController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *SystemRolloutController) processNextWorkItem() bool {
	key, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncSystemRollout()
	c.handleErr(err, key)

	return true
}

func (c *SystemRolloutController) syncSystemRollout() error {
	err := c.syncController()
	if err != nil {
		return err
	}

	defer func() {
		log := c.getLoggerForSystemRollout()
		systemRestore, err := c.ds.GetSystemRestore(c.systemRestoreName)
		if err != nil {
			log.WithError(err).Error("failed to get SystemRestore")
			close(c.stopCh)
		}

		notCompleted := systemRestore.Status.State != longhorn.SystemRestoreStateCompleted
		notFailed := systemRestore.Status.State != longhorn.SystemRestoreStateError
		if notCompleted && notFailed {
			log.Infof(SystemRolloutMsgRequeueNextPhaseFmt, systemRestore.Status.State)
			c.enqueue()
			return
		}

		_, err = c.ds.RemoveSystemRestoreLabel(systemRestore)
		if err != nil {
			message := fmt.Sprintf("failed to remove SystemRestore label after the restore state %v", systemRestore.Status.State)
			log.WithError(err).Warnf(SystemRolloutMsgRequeueDueToFmt, message)
			c.enqueue()
			return
		}

		log.Info("Shutting down")
		close(c.stopCh)
	}()

	return c.systemRollout()
}

func (c *SystemRolloutController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	c.logger.WithError(err).Warn("Worker error")
	c.queue.AddRateLimited(key)
}

func (c *SystemRolloutController) handleStatusUpdate(record *systemRolloutRecord, existing *longhorn.SystemRestore, err error, log logrus.FieldLogger) {
	if err != nil {
		c.cacheErrors.Append(util.NewMultiError(err.Error()))
	}

	if record.recordType == systemRolloutRecordTypeError && err != nil {
		c.cacheErrors.Append(util.NewMultiError(err.Error()))
	}

	if len(c.cacheErrors) != 0 {
		if record.reason == "" {
			record.reason = longhorn.SystemRestoreConditionReasonRestore
		}

		if record.message == "" {
			record.message = longhorn.SystemRestoreConditionMessageFailed
		}

		c.recordErrorState(record, c.systemRestore, c.cacheErrors.Join(), log)

	} else if record.recordType == systemRolloutRecordTypeNormal {
		c.recordNormalState(record, c.systemRestore, c.cacheErrors.Join(), log)
	}

	if !reflect.DeepEqual(existing.Status, c.systemRestore.Status) {
		_, err = c.ds.UpdateSystemRestoreStatus(c.systemRestore)
		if err != nil {
			log.WithError(err).Warnf(SystemRolloutMsgRequeueDueToFmt, "failed to update SystemRestore status")
			c.enqueue()
		}
	}
}

func (c *SystemRolloutController) getLoggerForSystemRollout() *logrus.Entry {
	log := c.logger.WithField("systemRestore", c.systemRestore.Name)

	if c.backupTargetURL != "" {
		log = log.WithField("backupTargetURL", c.backupTargetURL)
	}

	if c.engineImage != "" {
		log = log.WithField("engineImage", c.engineImage)
	}

	return log
}

func (c *SystemRolloutController) recordErrorState(record *systemRolloutRecord, systemRestore *longhorn.SystemRestore, err string, log logrus.FieldLogger) {
	systemRestore.Status.State = longhorn.SystemRestoreStateError
	systemRestore.Status.Conditions = types.SetCondition(
		systemRestore.Status.Conditions,
		longhorn.SystemRestoreConditionTypeError,
		longhorn.ConditionStatusTrue,
		record.reason,
		fmt.Sprintf("%v: %v", record.message, err),
	)

	log.WithError(fmt.Errorf(err)).Error(record.message)
	c.eventRecorder.Event(systemRestore, corev1.EventTypeWarning, constant.EventReasonFailed, util.CapitalizeFirstLetter(record.message))

	c.cacheErrors.Reset()
}

func (c *SystemRolloutController) recordNormalState(record *systemRolloutRecord, systemRestore *longhorn.SystemRestore, err string, log logrus.FieldLogger) {
	systemRestore.Status.State = record.nextState
	log.Info(record.message)
	c.eventRecorder.Event(systemRestore, corev1.EventTypeNormal, record.reason, record.message)
}

func (c *SystemRolloutController) updateSystemRolloutRecord(record *systemRolloutRecord, recordType systemRolloutRecordType, nextState longhorn.SystemRestoreState, reason, message string) {
	record.recordType = recordType
	record.nextState = nextState
	record.reason = reason
	record.message = message
}

func (c *SystemRolloutController) systemRollout() error {
	log := c.getLoggerForSystemRollout()

	systemRestore, err := c.ds.GetSystemRestore(c.systemRestoreName)
	if err != nil {
		return err
	}
	c.systemRestore = systemRestore

	if c.systemRestoredAt == "" {
		c.systemRestoredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if c.systemRestoredURL == "" {
		c.systemRestoredURL, err = systembackupstore.GetSystemBackupURL(c.systemRestore.Spec.SystemBackup, c.systemRestoreVersion, c.backupTargetURL)
		if err != nil {
			return err
		}
	}

	record := &systemRolloutRecord{}
	existingSystemRestore := c.systemRestore.DeepCopy()
	defer func() {
		c.handleStatusUpdate(record, existingSystemRestore, err, log)
	}()

	switch c.systemRestore.Status.State {
	case longhorn.SystemRestoreStatePending:
		err = c.initializeSystemRollout(log)
		if err != nil {
			return errors.Wrap(err, "failed to initialize system rollout")
		}

		c.updateSystemRolloutRecord(record,
			systemRolloutRecordTypeNormal, longhorn.SystemRestoreStateDownloading,
			constant.EventReasonStart, fmt.Sprintf(SystemRolloutMsgInitializedFmt, c.systemRestore.Spec.SystemBackup),
		)

	case longhorn.SystemRestoreStateDownloading:
		err = c.Download(log)
		if err != nil {
			return errors.Wrap(err, "failed to download system backup")
		}

		c.updateSystemRolloutRecord(record,
			systemRolloutRecordTypeNormal, longhorn.SystemRestoreStateUnpacking,
			constant.EventReasonFetched, fmt.Sprintf(SystemRolloutMsgDownloadedFmt, c.backupTargetURL),
		)

	case longhorn.SystemRestoreStateUnpacking:
		err = c.Unpack(log)
		if err != nil {
			c.updateSystemRolloutRecord(record,
				systemRolloutRecordTypeError, longhorn.SystemRestoreStateError,
				longhorn.SystemRestoreConditionReasonUnpack, longhorn.SystemRestoreConditionMessageUnpackFailed,
			)
			return nil
		}

		c.updateSystemRolloutRecord(record,
			systemRolloutRecordTypeNormal, longhorn.SystemRestoreStateRestoring,
			constant.EventReasonFetched, fmt.Sprintf(SystemRolloutMsgUnpackedFmt, c.downloadPath),
		)

	case longhorn.SystemRestoreStateRestoring:
		c.restore(types.APIExtensionsKindCustomResourceDefinitionList, c.restoreCustomResourceDefinitions, log)
		c.restore(types.KubernetesKindDaemonSetList, c.restoreDaemonSets, log)
		c.restore(types.LonghornKindEngineImageList, c.restoreEngineImages, log)
		c.restore(types.LonghornKindSettingList, c.restoreSettings, log)

		wg := &sync.WaitGroup{}
		restoreFns := map[string]func() error{
			types.KubernetesKindServiceList:               c.restoreService,
			types.KubernetesKindServiceAccountList:        c.restoreServiceAccounts,
			types.KubernetesKindClusterRoleList:           c.restoreClusterRoles,
			types.KubernetesKindClusterRoleBindingList:    c.restoreClusterRoleBindings,
			types.KubernetesKindPodSecurityPolicyList:     c.restorePodSecurityPolicies,
			types.KubernetesKindRoleList:                  c.restoreRoles,
			types.KubernetesKindRoleBindingList:           c.restoreRoleBindings,
			types.KubernetesKindStorageClassList:          c.restoreStorageClasses,
			types.KubernetesKindConfigMapList:             c.restoreConfigMaps,
			types.KubernetesKindDeploymentList:            c.restoreDeployments,
			types.LonghornKindVolumeList:                  c.restoreVolumes,
			types.KubernetesKindPersistentVolumeList:      c.restorePersistentVolumes,
			types.KubernetesKindPersistentVolumeClaimList: c.restorePersistentVolumeClaims,
			types.LonghornKindRecurringJobList:            c.restoreRecurringJobs,
		}
		wg.Add(len(restoreFns))
		for k, v := range restoreFns {
			kind, fn := k, v
			go func() {
				c.restore(kind, fn, log)
				wg.Done()
			}()
		}
		wg.Wait()

		if len(c.cacheErrors) == 0 {
			c.updateSystemRolloutRecord(record,
				systemRolloutRecordTypeNormal, longhorn.SystemRestoreStateCompleted,
				constant.EventReasonRestored, SystemRolloutMsgCompleted,
			)
		}
	}

	return nil
}

func (c *SystemRolloutController) initializeSystemRollout(log logrus.FieldLogger) error {
	c.systemRestore.Status.OwnerID = c.controllerID
	c.systemRestore.Status.State = longhorn.SystemRestoreStateInitializing

	systemBackupURL, err := c.GetSystemBackupURL()
	if err != nil {
		return err
	}
	c.systemRestore.Status.SourceURL = systemBackupURL

	c.systemRestore, err = c.ds.UpdateSystemRestore(c.systemRestore)
	if err != nil {
		return errors.Wrap(err, "failed to update SystemRestore")
	}

	return nil
}

func (c *SystemRolloutController) syncController() error {
	if c.systemRestore == nil {
		systemRestore, err := c.ds.GetSystemRestore(c.systemRestoreName)
		if err != nil {
			return err
		}

		c.downloadPath = filepath.Join(types.SystemRolloutDirTemp, systemRestore.Name+types.SystemBackupExtension)

		systemBackup, err := c.ds.GetSystemBackupRO(systemRestore.Spec.SystemBackup)
		if err != nil {
			return errors.Wrapf(err, "failed to get SystemBackup %v", systemRestore.Spec.SystemBackup)
		}

		currentLonghornVersion, err := c.ds.GetSetting(types.SettingNameCurrentLonghornVersion)
		if err != nil {
			return err
		}
		if systemBackup.Status.Version != currentLonghornVersion.Value {
			c.logger.WithFields(logrus.Fields{
				"from": currentLonghornVersion.Value,
				"to":   systemBackup.Status.Version,
			}).Warn("Restoring Longhorn to a different version")
		}
		c.systemRestoreVersion = systemBackup.Status.Version

		c.systemRestore = systemRestore
	}

	backupTarget, err := c.ds.GetDefaultBackupTargetRO()
	if err != nil {
		return err
	}

	backupTargetClient, err := newBackupTargetClientFromDefaultEngineImage(c.ds, backupTarget)
	if err != nil {
		return errors.Wrapf(err, "failed to init backup target clients")
	}

	systemBackupCfg, err := backupTargetClient.GetSystemBackupConfig(c.systemRestore.Spec.SystemBackup, c.systemRestoreVersion)
	if err != nil {
		return err
	}

	obj, err := c.createSystemBackupEngineImage(systemBackupCfg.EngineImage)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	engineImage, ok := obj.(*longhorn.EngineImage)
	if !ok {
		return fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, obj.GetObjectKind(), types.LonghornKindEngineImage)
	}

	if engineImage.Status.State != longhorn.EngineImageStateDeployed {
		return errors.Errorf("engine image %v not %v yet", engineImage.Name, longhorn.EngineImageStateDeployed)
	}

	rolloutBackupTargetClient, err := newBackupTargetClient(c.ds, backupTarget, systemBackupCfg.EngineImage)
	if err != nil {
		return errors.Wrapf(err, "failed to init rollout backup target clients")
	}

	c.backupTargetClient = rolloutBackupTargetClient
	c.backupTargetCredential = backupTargetClient.Credential
	c.backupTargetURL = backupTargetClient.URL
	c.engineImage = systemBackupCfg.EngineImage

	c.cacheErrors = util.MultiError{}

	return nil
}

func (c *SystemRolloutController) createSystemBackupEngineImage(engineImage string) (runtime.Object, error) {
	obj, err := c.ds.GetEngineImageByImage(engineImage)
	if err != nil {
		if !types.ErrorIsNotFound(err) {
			return nil, err
		}
	}
	if obj != nil {
		return obj, nil
	}

	engineImageName := types.GetEngineImageChecksumName(engineImage)

	log := c.logger.WithField(types.LonghornKindEngineImage, engineImageName)
	log.Info("Creating EngineImage for system rollout")

	newEngineImage := &longhorn.EngineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:   engineImageName,
			Labels: types.GetEngineImageLabels(engineImageName),
		},
		Spec: longhorn.EngineImageSpec{
			Image: engineImage,
		},
	}
	fnCreate := func(restore runtime.Object) (runtime.Object, error) {
		obj, ok := restore.(*longhorn.EngineImage)
		if !ok {
			return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.LonghornKindEngineImage)
		}
		return c.ds.CreateEngineImage(obj)
	}
	return c.rolloutResource(newEngineImage, fnCreate, false, log, SystemRolloutMsgRestoredItem)
}

func (c *SystemRolloutController) cacheKubernetesResources() error {
	scheme := kubernetesscheme.Scheme
	return c.cacheResourcesFromDirectory(c.getYAMLDirectory(types.SystemBackupSubDirKubernetes), scheme)
}

func (c *SystemRolloutController) cacheAPIExtensionsResources() error {
	scheme := runtime.NewScheme()
	err := apiextensionsv1.SchemeBuilder.AddToScheme(scheme)
	if err != nil {
		return err
	}
	return c.cacheResourcesFromDirectory(c.getYAMLDirectory(types.SystemBackupSubDirAPIExtensions), scheme)
}

func (c *SystemRolloutController) cacheLonghornResources() error {
	scheme := runtime.NewScheme()
	err := longhorn.SchemeBuilder.AddToScheme(scheme)
	if err != nil {
		return err
	}
	return c.cacheResourcesFromDirectory(c.getYAMLDirectory(types.SystemBackupSubDirLonghorn), scheme)
}

func (c *SystemRolloutController) cacheResourcesFromDirectory(name string, scheme *runtime.Scheme) error {
	codecs := serializer.NewCodecFactory(scheme)

	files, err := os.ReadDir(name)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}

		return errors.Wrapf(err, "failed to read directory %v", name)
	}

	for _, f := range files {
		if f.IsDir() {
			logrus.Debugf("%v is a directory", f.Name())
			continue
		}

		path := filepath.Join(name, f.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to read file %v", path)
			continue
		}

		decode := codecs.UniversalDeserializer().Decode
		obj, gvk, err := decode(contents, nil, nil)
		if err != nil {
			return err
		}

		switch gvk.Kind {
		// API Extensions
		case types.APIExtensionsKindCustomResourceDefinitionList:
			c.customResourceDefinitionList = obj.(*apiextensionsv1.CustomResourceDefinitionList)
		// Kubernetes RBAC
		case types.KubernetesKindClusterRoleList:
			c.clusterRoleList = obj.(*rbacv1.ClusterRoleList)
		case types.KubernetesKindClusterRoleBindingList:
			c.clusterRoleBindingList = obj.(*rbacv1.ClusterRoleBindingList)
		case types.KubernetesKindRoleList:
			c.roleList = obj.(*rbacv1.RoleList)
		case types.KubernetesKindRoleBindingList:
			c.roleBindingList = obj.(*rbacv1.RoleBindingList)
		// Kubernetes Apps
		case types.KubernetesKindDaemonSetList:
			c.daemonSetList = obj.(*appsv1.DaemonSetList)
		case types.KubernetesKindDeploymentList:
			c.deploymentList = obj.(*appsv1.DeploymentList)
		// Kubernetes Core
		case types.KubernetesKindPersistentVolumeList:
			c.persistentVolumeList = obj.(*corev1.PersistentVolumeList)
		case types.KubernetesKindPersistentVolumeClaimList:
			c.persistentVolumeClaimList = obj.(*corev1.PersistentVolumeClaimList)
		case types.KubernetesKindServiceAccountList:
			c.serviceAccountList = obj.(*corev1.ServiceAccountList)
		case types.KubernetesKindServiceList:
			c.serviceList = obj.(*corev1.ServiceList)
		case types.KubernetesKindConfigMapList:
			c.configMapList = obj.(*corev1.ConfigMapList)
		// Kubernetes Storage
		case types.KubernetesKindStorageClassList:
			c.storageClassList = obj.(*storagev1.StorageClassList)
		// Kubernetes Policy
		case types.KubernetesKindPodSecurityPolicyList:
			c.podSecurityPolicyList = obj.(*policyv1beta1.PodSecurityPolicyList)
		// Longhorn
		case types.LonghornKindEngineImageList:
			c.engineImageList = obj.(*longhorn.EngineImageList)
		case types.LonghornKindRecurringJobList:
			c.recurringJobList = obj.(*longhorn.RecurringJobList)
		case types.LonghornKindSettingList:
			c.settingList = obj.(*longhorn.SettingList)
		case types.LonghornKindVolumeList:
			c.volumeList = obj.(*longhorn.VolumeList)
		default:
			log := c.getLoggerForSystemRollout()
			log.Errorf("Unknown resource kind %v", gvk.Kind)
		}
	}
	return nil
}

func (c *SystemRolloutController) getYAMLDirectory(name string) string {
	return filepath.Join(filepath.Dir(c.downloadPath), c.systemRestore.Spec.SystemBackup, types.SystemBackupSubDirYaml, name)
}

func (c *SystemRolloutController) Download(log logrus.FieldLogger) error {
	err := c.backupTargetClient.DownloadSystemBackup(c.systemRestore.Spec.SystemBackup, c.systemRestoreVersion, c.downloadPath)
	if err != nil {
		return err
	}

	_, err = os.Stat(c.downloadPath)
	if err != nil {
		return err
	}

	return nil
}

func (c *SystemRolloutController) Unpack(log logrus.FieldLogger) error {
	cmd := exec.Command("unzip", c.downloadPath)
	cmd.Dir = filepath.Dir(c.downloadPath)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to unzip %v", c.downloadPath)
	}

	if err := c.cacheKubernetesResources(); err != nil {
		return errors.Wrap(err, "failed to extract Kubernetes resources")
	}

	if err := c.cacheAPIExtensionsResources(); err != nil {
		return errors.Wrap(err, "failed to extract API Extensions resources")
	}

	if err := c.cacheLonghornResources(); err != nil {
		return errors.Wrap(err, "failed to extract Longhorn resources")
	}

	return nil
}

func (c *SystemRolloutController) GetSystemBackupURL() (string, error) {
	log := c.getLoggerForSystemRollout()

	systemBackups, err := c.backupTargetClient.ListSystemBackup()
	if err != nil {
		return "", errors.Wrap(err, "failed to list rollouts in the backup target")
	}

	for name, uri := range systemBackups {
		systemBackupName := string(name)
		systemBackupURI := string(uri)

		if systemBackupName != c.systemRestore.Spec.SystemBackup {
			continue
		}

		systemBackupVersion, _, err := parseSystemBackupURI(systemBackupURI)
		if err != nil {
			return "", err
		}

		if systemBackupVersion != c.systemRestoreVersion {
			log.Debugf("Found %v version in mismatching version %v, expecting %v", name, systemBackupVersion, c.systemRestoreVersion)
			continue
		}

		return c.backupTargetURL + systemBackupURI, nil
	}

	return "", errors.Errorf("cannot find system backup %v of version %v in %v", c.systemRestore.Spec.SystemBackup, c.systemRestoreVersion, systemBackups)
}

func (c *SystemRolloutController) restore(kind string, fn func() error, log logrus.FieldLogger) {
	timeout := time.Duration(datastore.SystemRestoreTimeout) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var err error
	for {
		select {
		case <-timer.C:
			c.postRestoreHandle(kind, err)
			return
		case <-ticker.C:
			err = fn()
			if err == nil {
				c.postRestoreHandle(kind, nil)
				return
			}
			log.WithError(err).Debugf(SystemRolloutMsgRestoringFmt, kind)
		}
	}
}

func (c *SystemRolloutController) postRestoreHandle(kind string, restoreError error) {
	log := c.getLoggerForSystemRollout()

	systemRestore, err := c.ds.GetSystemRestoreInProgress(c.systemRestoreName)
	if err != nil {
		c.cacheErrors.Append(util.NewMultiError(err.Error()))
		return
	}

	if systemRestore.Status.State == longhorn.SystemRestoreStateError {
		return
	}

	if restoreError != nil {
		c.cacheErrors.Append(util.NewMultiError(restoreError.Error()))
		return
	}

	restoredMessage := fmt.Sprintf(SystemRolloutMsgRestoredKindFmt, kind)
	log.Info(restoredMessage)
	c.eventRecorder.Event(systemRestore, corev1.EventTypeNormal, fmt.Sprintf(constant.EventReasonRestoredFmt, kind), restoredMessage)
}

func (c *SystemRolloutController) rolloutResource(obj runtime.Object, fnRollout func(runtime.Object) (runtime.Object, error), isSkipped bool, log logrus.FieldLogger, message string) (runtime.Object, error) {
	err := c.tagLonghornLastSystemRestoreAnnotation(obj, isSkipped, log, message)
	if err != nil {
		if types.ErrorAlreadyExists(err) {
			return obj, nil
		}
		return nil, err
	}

	return fnRollout(obj)
}

func (c *SystemRolloutController) restoreClusterRoles() (err error) {
	if c.clusterRoleList == nil {
		return nil
	}

	for _, restore := range c.clusterRoleList.Items {
		log := c.logger.WithField(types.KubernetesKindClusterRole, restore.Name)

		exist, err := c.ds.GetClusterRole(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*rbacv1.ClusterRole)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindClusterRole)
				}
				return c.ds.CreateClusterRole(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Rules, restore.Rules) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Rules = restore.Rules

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*rbacv1.ClusterRole)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindClusterRole)
			}
			return c.ds.UpdateClusterRole(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreClusterRoleBindings() (err error) {
	if c.clusterRoleBindingList == nil {
		return nil
	}

	for _, restore := range c.clusterRoleBindingList.Items {
		log := c.logger.WithField(types.KubernetesKindClusterRoleBinding, restore.Name)

		exist, err := c.ds.GetClusterRoleBinding(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*rbacv1.ClusterRoleBinding)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindClusterRoleBinding)
				}
				return c.ds.CreateClusterRoleBinding(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.RoleRef, restore.RoleRef) || !reflect.DeepEqual(exist.Subjects, restore.Subjects) {
			log.Info(SystemRolloutMsgUpdating)
			exist.RoleRef = restore.RoleRef
			exist.Subjects = restore.Subjects

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*rbacv1.ClusterRoleBinding)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindClusterRoleBinding)
			}
			return c.ds.UpdateClusterRoleBinding(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreConfigMaps() (err error) {
	if c.configMapList == nil {
		return nil
	}

	for _, restore := range c.configMapList.Items {
		log := c.logger.WithField(types.KubernetesKindConfigMap, restore.Name)

		if restore.Name == types.DefaultDefaultSettingConfigMapName {
			log.Infof(SystemRolloutMsgIgnoreItemFmt, types.DefaultDefaultSettingConfigMapName)
			continue
		}

		exist, err := c.ds.GetConfigMapRO(restore.Namespace, restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*corev1.ConfigMap)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindConfigMap)
				}
				return c.ds.CreateConfigMap(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Data, restore.Data) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Data = restore.Data

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*corev1.ConfigMap)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindConfigMap)
			}
			return c.ds.UpdateConfigMap(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreCustomResourceDefinitions() (err error) {
	if c.customResourceDefinitionList == nil {
		return nil
	}

	for _, restore := range c.customResourceDefinitionList.Items {
		log := c.logger.WithField(types.APIExtensionsKindCustomResourceDefinition, restore.Name)

		exist, err := c.ds.GetCustomResourceDefinition(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*apiextensionsv1.CustomResourceDefinition)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.APIExtensionsKindCustomResourceDefinition)
				}
				return c.ds.CreateCustomResourceDefinition(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		existVersions := map[string]apiextensionsv1.CustomResourceDefinitionVersion{}
		for _, version := range exist.Spec.Versions {
			existVersions[version.Name] = version
		}

		updateExist := exist.DeepCopy()
		for _, restoreVersion := range restore.Spec.Versions {
			_, found := existVersions[restoreVersion.Name]
			if !found {
				updateExist.Spec.Versions = append(updateExist.Spec.Versions, restoreVersion)

				log.WithFields(logrus.Fields{
					"CRD":     restore.Name,
					"version": restoreVersion.Name,
				}).Debug("Adding CustomResourceDefinition version")

				continue
			}

			for i, existVersion := range updateExist.Spec.Versions {
				if existVersion.Name != restoreVersion.Name {
					continue
				}

				if !reflect.DeepEqual(existVersion, restoreVersion) {
					updateExist.Spec.Versions[i] = restoreVersion

					log.WithFields(logrus.Fields{
						"CRD":     updateExist.Name,
						"version": existVersion.Name,
					}).Debug("Updating CustomResourceDefinition version")
				}
				break
			}
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec.Versions, updateExist.Spec.Versions) {
			log.Info(SystemRolloutMsgUpdating)

			isSkipped = false
		}
		fnUpdate := func(updateExist runtime.Object) (runtime.Object, error) {
			obj, ok := updateExist.(*apiextensionsv1.CustomResourceDefinition)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.APIExtensionsKindCustomResourceDefinition)
			}
			return c.ds.UpdateCustomResourceDefinition(obj)
		}
		_, err = c.rolloutResource(updateExist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreEngineImages() (err error) {
	if c.engineImageList == nil {
		return nil
	}

	for _, restore := range c.engineImageList.Items {
		log := c.logger.WithField(types.LonghornKindEngineImage, restore.Name)

		exist, err := c.ds.GetEngineImage(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*longhorn.EngineImage)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.LonghornKindEngineImage)
				}
				return c.ds.CreateEngineImage(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*longhorn.EngineImage)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.LonghornKindEngineImage)
			}
			return c.ds.UpdateEngineImage(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreDaemonSets() (err error) {
	if c.daemonSetList == nil {
		return nil
	}

	for _, restore := range c.daemonSetList.Items {
		log := c.logger.WithField(types.KubernetesKindDaemonSet, restore.Name)

		exist, err := c.ds.GetDaemonSet(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*appsv1.DaemonSet)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindDaemonSet)
				}
				return c.ds.CreateDaemonSet(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}

			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*appsv1.DaemonSet)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindDaemonSet)
			}
			return c.ds.UpdateDaemonSet(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreDeployments() (err error) {
	if c.deploymentList == nil {
		return nil
	}

	for _, restore := range c.deploymentList.Items {
		log := c.logger.WithField(types.KubernetesKindDeployment, restore.Name)

		exist, err := c.ds.GetDeployment(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*appsv1.Deployment)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindDeployment)
				}
				return c.ds.CreateDeployment(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*appsv1.Deployment)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindDeployment)
			}
			return c.ds.UpdateDeployment(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restorePersistentVolumes() (err error) {
	if c.persistentVolumeList == nil {
		return nil
	}

	for _, restore := range c.persistentVolumeList.Items {
		notResourceRestoreRequired, err := c.isResourceHasCurrentRolloutAnnotation(&restore)
		if err != nil {
			return err
		}
		if notResourceRestoreRequired {
			continue
		}

		log := c.logger.WithField(types.KubernetesKindPersistentVolume, restore.Name)

		exist, err := c.ds.GetPersistentVolume(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			volume, err := c.ds.GetVolumeRO(restore.Spec.CSI.VolumeHandle)
			if err != nil {
				return err
			}

			restoreCondition := types.GetCondition(volume.Status.Conditions, longhorn.VolumeConditionTypeRestore)
			if restoreCondition.Status == longhorn.ConditionStatusTrue {
				return errors.Errorf("volume is restoring data")
			}

			if volume.Status.RestoreRequired {
				return errors.Errorf("volume is waiting to restore data")
			}

			// Remove ClaimRef to reuse the persistent volume resource.
			restore.Spec.ClaimRef = nil
			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*corev1.PersistentVolume)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindPersistentVolume)
				}
				return c.ds.CreatePersistentVolume(obj)
			}
			_, err = c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*corev1.PersistentVolume)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindPersistentVolume)
			}
			return c.ds.UpdatePersistentVolume(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, true, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restorePersistentVolumeClaims() (err error) {
	if c.persistentVolumeClaimList == nil {
		return nil
	}

	for _, restore := range c.persistentVolumeClaimList.Items {
		notResourceRestoreRequired, err := c.isResourceHasCurrentRolloutAnnotation(&restore)
		if err != nil {
			return err
		}
		if notResourceRestoreRequired {
			continue
		}

		log := c.logger.WithField(types.KubernetesKindPersistentVolumeClaim, restore.Name)

		exist, err := c.ds.GetPersistentVolumeClaim(restore.Namespace, restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			_, err := c.ds.GetPersistentVolumeRO(restore.Spec.VolumeName)
			if err != nil {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*corev1.PersistentVolumeClaim)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindPersistentVolumeClaim)
				}
				return c.ds.CreatePersistentVolumeClaim(obj.GetNamespace(), obj)
			}
			_, err = c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*corev1.PersistentVolumeClaim)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindPersistentVolumeClaim)
			}
			return c.ds.UpdatePersistentVolumeClaim(obj.GetNamespace(), obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, true, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restorePodSecurityPolicies() (err error) {
	if c.podSecurityPolicyList == nil {
		return nil
	}

	for _, restore := range c.podSecurityPolicyList.Items {
		log := c.logger.WithField(types.KubernetesKindPodSecurityPolicy, restore.Name)

		exist, err := c.ds.GetPodSecurityPolicy(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*policyv1beta1.PodSecurityPolicy)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindPodSecurityPolicy)
				}
				return c.ds.CreatePodSecurityPolicy(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*policyv1beta1.PodSecurityPolicy)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindPodSecurityPolicy)
			}
			return c.ds.UpdatePodSecurityPolicy(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreRecurringJobs() (err error) {
	if c.recurringJobList == nil {
		return nil
	}

	for _, restore := range c.recurringJobList.Items {
		log := c.logger.WithField(types.LonghornKindRecurringJob, restore.Name)

		exist, err := c.ds.GetRecurringJob(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*longhorn.RecurringJob)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.LonghornKindRecurringJob)
				}
				return c.ds.CreateRecurringJob(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*longhorn.RecurringJob)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.LonghornKindRecurringJob)
			}
			return c.ds.UpdateRecurringJob(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreRoles() (err error) {
	if c.roleList == nil {
		return nil
	}

	for _, restore := range c.roleList.Items {
		log := c.logger.WithField(types.KubernetesKindRole, restore.Name)

		exist, err := c.ds.GetRole(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*rbacv1.Role)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindRole)
				}
				return c.ds.CreateRole(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Rules, restore.Rules) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Rules = restore.Rules

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*rbacv1.Role)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindRole)
			}
			return c.ds.UpdateRole(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreRoleBindings() (err error) {
	if c.roleBindingList == nil {
		return nil
	}

	for _, restore := range c.roleBindingList.Items {
		log := c.logger.WithField(types.KubernetesKindRoleBinding, restore.Name)

		exist, err := c.ds.GetRoleBinding(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*rbacv1.RoleBinding)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindRoleBinding)
				}
				return c.ds.CreateRoleBinding(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.RoleRef, restore.RoleRef) || !reflect.DeepEqual(exist.Subjects, restore.Subjects) {
			log.Info(SystemRolloutMsgUpdating)
			exist.RoleRef = restore.RoleRef
			exist.Subjects = restore.Subjects

			isSkipped = false
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*rbacv1.RoleBinding)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindRoleBinding)
			}
			return c.ds.UpdateRoleBinding(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreService() (err error) {
	if c.serviceList == nil {
		return nil
	}

	for _, restore := range c.serviceList.Items {
		log := c.logger.WithField(types.KubernetesKindService, restore.Name)

		exist, err := c.ds.GetService(restore.Namespace, restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*corev1.Service)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindService)
				}
				return c.ds.CreateService(obj.GetNamespace(), obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if !reflect.DeepEqual(exist.Spec, restore.Spec) {
			log.Info(SystemRolloutMsgUpdating)
			exist.Spec = restore.Spec

			isSkipped = false
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*corev1.Service)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindService)
			}
			return c.ds.UpdateService(obj.GetNamespace(), obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreServiceAccounts() (err error) {
	if c.serviceAccountList == nil {
		return nil
	}

	for _, restore := range c.serviceAccountList.Items {
		log := c.logger.WithField(types.KubernetesKindServiceAccount, restore.Name)

		exist, err := c.ds.GetServiceAccount(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*corev1.ServiceAccount)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindServiceAccount)
				}
				return c.ds.CreateServiceAccount(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*corev1.ServiceAccount)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindServiceAccount)
			}
			return c.ds.UpdateServiceAccount(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, true, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

var systemRolloutIgnoredSettings = [...]string{
	string(types.SettingNameConcurrentBackupRestorePerNodeLimit),
	string(types.SettingNameConcurrentReplicaRebuildPerNodeLimit),
	string(types.SettingNameBackupTarget),
	string(types.SettingNameBackupTargetCredentialSecret),
}

func isSystemRolloutIgnoredSetting(name string) bool {
	for _, ignoredSetting := range systemRolloutIgnoredSettings {
		if name == ignoredSetting {
			return true
		}
	}
	return false
}

func (c *SystemRolloutController) restoreSettings() (err error) {
	if c.settingList == nil {
		return nil
	}

	for _, restore := range c.settingList.Items {
		log := c.logger.WithField(types.LonghornKindSetting, restore.Name)

		if isSystemRolloutIgnoredSetting(restore.Name) {
			log.Infof(SystemRolloutMsgIgnoreItemFmt, "this configurable setting persists through the restore")
			continue
		}

		exist, err := c.ds.GetSettingExact(types.SettingName(restore.Name))
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*longhorn.Setting)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.LonghornKindSetting)
				}
				return c.ds.CreateSetting(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		isSkipped := true
		if exist.Value != restore.Value {
			log.Info(SystemRolloutMsgUpdating)
			exist.Value = restore.Value

			isSkipped = false
		}
		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*longhorn.Setting)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.LonghornKindSetting)
			}
			return c.ds.UpdateSetting(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, isSkipped, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreStorageClasses() (err error) {
	if c.storageClassList == nil {
		return nil
	}

	for _, restore := range c.storageClassList.Items {
		log := c.logger.WithField(types.KubernetesKindStorageClass, restore.Name)

		exist, err := c.ds.GetStorageClass(restore.Name)
		if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*storagev1.StorageClass)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.KubernetesKindStorageClass)
				}
				return c.ds.CreateStorageClass(obj)
			}
			_, err = c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
			continue
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*storagev1.StorageClass)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.KubernetesKindStorageClass)
			}
			return c.ds.UpdateStorageClass(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, true, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) restoreVolumes() (err error) {
	if c.engineImageList != nil {
		for _, restoreEngineImage := range c.engineImageList.Items {
			obj, err := c.ds.GetLonghornEngineImage(restoreEngineImage.Name)
			if err != nil {
				return err
			}

			ei, ok := obj.(*longhorn.EngineImage)
			if !ok {
				return fmt.Errorf("BUG: cannot convert %v to EngineImage object", restoreEngineImage.Name)
			}

			if !ei.DeletionTimestamp.IsZero() {
				return errors.Errorf("engine image is deleting")
			}

			if ei.Status.State != longhorn.EngineImageStateDeployed {
				return errors.Errorf("engine image is in %v state", ei.Status.State)
			}
		}
	}

	if c.volumeList == nil {
		return nil
	}

	for _, restore := range c.volumeList.Items {
		log := c.logger.WithField(types.LonghornKindVolume, restore.Name)
		log = getLoggerForVolume(log, &restore)

		exist, err := c.ds.GetVolume(restore.Name)
		if err == nil && exist != nil && exist.Spec.NodeID != "" {
			log.Warn("Cannot restore attached volume")
			continue

		} else if err != nil {
			if !datastore.ErrorIsNotFound(err) {
				return err
			}

			restore.ResourceVersion = ""
			restore.Spec.NodeID = ""

			if restore.Status.LastBackup != "" {
				restore.Spec.FromBackup = backupstore.EncodeBackupURL(restore.Status.LastBackup, restore.Name, c.backupTargetURL)
				log = log.WithField("fromBackup", restore.Spec.FromBackup)
			}

			if err = tagLonghornLastSystemRestoreBackupAnnotation(restore.Spec.FromBackup, &restore); err != nil {
				return err
			}

			log.Info(SystemRolloutMsgCreating)

			fnCreate := func(restore runtime.Object) (runtime.Object, error) {
				obj, ok := restore.(*longhorn.Volume)
				if !ok {
					return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, restore.GetObjectKind(), types.LonghornKindVolume)
				}
				return c.ds.CreateVolume(obj)
			}
			_, err := c.rolloutResource(&restore, fnCreate, false, log, SystemRolloutMsgRestoredItem)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				err = errors.Wrapf(err, SystemRolloutErrFailedToCreateFmt, types.LonghornKindVolume, restore.Name)
				message := util.CapitalizeFirstLetter(err.Error())
				log.Warn(message)

				reason := fmt.Sprintf(constant.EventReasonFailedCreatingFmt, types.LonghornKindVolume, restore.Name)
				c.eventRecorder.Event(c.systemRestore, corev1.EventTypeWarning, reason, message)

				if err = c.ignorePersistenVolumeDueToMissingVolume(&restore); err != nil {
					return err
				}

				if err = c.ignorePersistenVolumeClaimDueToMissingVolume(&restore); err != nil {
					return err
				}
			}
			continue
		}

		fnUpdate := func(exist runtime.Object) (runtime.Object, error) {
			obj, ok := exist.(*longhorn.Volume)
			if !ok {
				return nil, fmt.Errorf(SystemRolloutErrFailedConvertToObjectFmt, exist.GetObjectKind(), types.LonghornKindVolume)
			}
			return c.ds.UpdateVolume(obj)
		}
		_, err = c.rolloutResource(exist, fnUpdate, true, log, SystemRolloutMsgSkipIdentical)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SystemRolloutController) ignorePersistenVolumeDueToMissingVolume(volume *longhorn.Volume) error {
	newPersistentVolumeListItems := []corev1.PersistentVolume{}
	for _, persistenVolume := range c.persistentVolumeList.Items {
		if persistenVolume.Spec.CSI.VolumeHandle == volume.Name {
			log := c.logger.WithField(types.KubernetesKindPersistentVolume, persistenVolume.Name)
			message := fmt.Sprintf(SystemRolloutMsgIgnoreItemFmt,
				fmt.Sprintf(SystemRolloutErrMissingDependencyFmt, persistenVolume.Name, types.LonghornKindVolume, volume.Name),
			)
			err := c.tagLonghornLastSystemRestoreAnnotation(&persistenVolume, true, log, message)
			if err != nil && !types.ErrorAlreadyExists(err) {
				return err
			}

			reason := fmt.Sprintf(constant.EventReasonRolloutSkippedFmt, types.KubernetesKindPersistentVolume, persistenVolume.Name)
			c.eventRecorder.Event(c.systemRestore, corev1.EventTypeWarning, reason, message)
		}
		newPersistentVolumeListItems = append(newPersistentVolumeListItems, persistenVolume)
	}

	c.persistentVolumeList.Items = newPersistentVolumeListItems
	return nil
}

func (c *SystemRolloutController) ignorePersistenVolumeClaimDueToMissingVolume(volume *longhorn.Volume) error {
	newPersistentVolumeClaimListItems := []corev1.PersistentVolumeClaim{}
	for _, persistenVolumeClaim := range c.persistentVolumeClaimList.Items {
		if persistenVolumeClaim.Spec.VolumeName == volume.Name {
			log := c.logger.WithField(types.KubernetesKindPersistentVolumeClaim, persistenVolumeClaim.Name)
			message := fmt.Sprintf(SystemRolloutMsgIgnoreItemFmt,
				fmt.Sprintf(SystemRolloutErrMissingDependencyFmt, persistenVolumeClaim.Name, types.LonghornKindVolume, volume.Name),
			)
			err := c.tagLonghornLastSystemRestoreAnnotation(&persistenVolumeClaim, true, log, message)
			if err != nil && !types.ErrorAlreadyExists(err) {
				return err
			}

			reason := fmt.Sprintf(constant.EventReasonRolloutSkippedFmt, types.KubernetesKindPersistentVolumeClaim, persistenVolumeClaim.Name)
			c.eventRecorder.Event(c.systemRestore, corev1.EventTypeWarning, reason, message)
		}
		newPersistentVolumeClaimListItems = append(newPersistentVolumeClaimListItems, persistenVolumeClaim)
	}

	c.persistentVolumeClaimList.Items = newPersistentVolumeClaimListItems
	return nil
}

func (c *SystemRolloutController) isResourceHasCurrentRolloutAnnotation(obj runtime.Object) (bool, error) {
	keys := []string{
		types.GetLastSystemRestoreAtLabelKey(),
		types.GetLastSkippedSystemRestoreAtLabelKey(),
	}

	for _, key := range keys {
		systemRestoreAt, err := util.GetAnnotation(obj, key)
		if err != nil {
			return false, err
		}

		if systemRestoreAt == c.systemRestoredAt {
			return true, nil
		}
	}
	return false, nil
}

func (c *SystemRolloutController) tagLonghornLastSystemRestoreAnnotation(obj runtime.Object, isSkipped bool, log logrus.FieldLogger, message string) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	annos := metadata.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}

	isAnnotated, err := c.isResourceHasCurrentRolloutAnnotation(obj)
	if err != nil {
		return err
	}
	if isAnnotated {
		return fmt.Errorf("system restore annotation already exists")
	}

	if isSkipped {
		delete(annos, types.GetLastSystemRestoreLabelKey())
		delete(annos, types.GetLastSystemRestoreAtLabelKey())

		annos[types.GetLastSkippedSystemRestoreLabelKey()] = c.systemRestoredURL
		annos[types.GetLastSkippedSystemRestoreAtLabelKey()] = c.systemRestoredAt
	} else {
		delete(annos, types.GetLastSkippedSystemRestoreLabelKey())
		delete(annos, types.GetLastSkippedSystemRestoreAtLabelKey())

		annos[types.GetLastSystemRestoreLabelKey()] = c.systemRestoredURL
		annos[types.GetLastSystemRestoreAtLabelKey()] = c.systemRestoredAt
	}

	log.Debug(message)
	metadata.SetAnnotations(annos)
	return nil
}

func tagLonghornLastSystemRestoreBackupAnnotation(lastRolloutBackup string, obj runtime.Object) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	annos := metadata.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos[types.GetLastSystemRestoreBackupLabelKey()] = lastRolloutBackup
	metadata.SetAnnotations(annos)
	return nil
}

func getSystemRolloutName(systemRestoreName string) string {
	return SystemRolloutNamePrefix + systemRestoreName
}

package controller

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	systembackupstore "github.com/longhorn/backupstore/systembackup"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SystemBackupControllerName = "longhorn-system-backup"

	SystemBackupTempDir = "/tmp"

	SystemBackupErrArchive       = "failed to archive system backup file"
	SystemBackupErrDelete        = "failed to delete system backup in backup target"
	SystemBackupErrGenerate      = "failed to generate system backup file"
	SystemBackupErrGetFmt        = "failed to get %v"
	SystemBackupErrGetConfig     = "failed to get system backup config"
	SystemBackupErrMkdir         = "failed to create system backup file directory"
	SystemBackupErrRemoveAll     = "failed to remove system backup directory"
	SystemBackupErrRemove        = "failed to remove system backup file"
	SystemBackupErrOSStat        = "failed to compute system backup file size"
	SystemBackupErrSync          = "failed to sync from backup target"
	SystemBackupErrTimeoutUpload = "timeout uploading system backup"
	SystemBackupErrUpload        = "failed to upload system backup file"
	SystemBackupErrGenerateYAML  = "failed to generate resource YAMLs"

	SystemBackupMsgCreatedArchieveFmt  = "Created system backup file: %v"
	SystemBackupMsgDeletingRemote      = "Deleting system backup in backup target"
	SystemBackupMsgDeleted             = "Deleted system backup"
	SystemBackupMsgRequeueNextPhaseFmt = "Requeue %v for next phase: %v"
	SystemBackupMsgStarting            = "Starting system backup"
	SystemBackupMsgSyncedBackupTarget  = "Synced system backup from backup target"
	SystemBackupMsgSyncingBackupTarget = "Syncing system backup from backup target"
	SystemBackupMsgUploadBackupTarget  = "Uploaded system backup to backup target"
)

type systemBackupRecordType string

const (
	systemBackupRecordTypeError  = systemBackupRecordType("error")
	systemBackupRecordTypeNone   = systemBackupRecordType("")
	systemBackupRecordTypeNormal = systemBackupRecordType("normal")
)

type systemBackupRecord struct {
	nextState longhorn.SystemBackupState

	recordType systemBackupRecordType
	message    string
	reason     string
}

type SystemBackupController struct {
	*baseController

	// which namespace controller is running with
	namespace string

	// use as the OwnerID of the controller
	controllerID string

	// the running manager image
	managerImage string

	kubeClient clientset.Interface

	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewSystemBackupController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace string,
	controllerID string,
	managerImage string) *SystemBackupController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	c := &SystemBackupController{
		baseController: newBaseController(SystemBackupControllerName, logger),

		namespace:    namespace,
		controllerID: controllerID,
		managerImage: managerImage,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: SystemBackupControllerName + "-controller"}),
	}

	ds.SystemBackupInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		UpdateFunc: func(old, cur interface{}) { c.enqueue(cur) },
		DeleteFunc: c.enqueue,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.SystemBackupInformer.HasSynced)

	return c
}

func (c *SystemBackupController) enqueue(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	c.queue.Add(key)
}

func (c *SystemBackupController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting Longhorn SystemBackup controller")
	defer c.logger.Info("Shut down Longhorn SystemBackup controller")

	if !cache.WaitForNamedCacheSync(c.name, stopCh, c.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (c *SystemBackupController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *SystemBackupController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncSystemBackup(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *SystemBackupController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	log := c.logger.WithField("systemBackup", key)

	if c.queue.NumRequeues(key) < maxRetries {
		log.WithError(err).Warn("Failed to sync Longhorn SystemBackup, and requeuing to reconcile")

		c.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	log.WithError(err).Warn("Failed to sync Longhorn systemBackup, and dropping it out of the queue")
	c.queue.Forget(key)
}

func (c *SystemBackupController) handleStatusUpdate(record *systemBackupRecord, systemBackup *longhorn.SystemBackup, existingSystemBackup *longhorn.SystemBackup, err error, log logrus.FieldLogger) {
	switch record.recordType {
	case systemBackupRecordTypeError:
		c.recordErrorState(record, systemBackup)
	case systemBackupRecordTypeNormal:
		c.recordNormalState(record, systemBackup)
	default:
		return
	}

	if !reflect.DeepEqual(existingSystemBackup.Status, systemBackup.Status) {
		systemBackup.Status.LastSyncedAt = metav1.Time{Time: time.Now().UTC()}
		if _, err = c.ds.UpdateSystemBackupStatus(systemBackup); err != nil {
			log.WithError(err).Debugf("Requeue %v due to error", systemBackup.Name)
			c.enqueue(systemBackup)
			err = nil
			return
		}
	}

	switch record.recordType {
	case systemBackupRecordTypeError:
		c.LogErrorState(record, systemBackup, log)
	case systemBackupRecordTypeNormal:
		c.LogNormalState(record, systemBackup, log)
	default:
		return
	}

	if systemBackup.Status.State != existingSystemBackup.Status.State {
		log.Infof(SystemBackupMsgRequeueNextPhaseFmt, systemBackup.Name, systemBackup.Status.State)
	}
}

func (c *SystemBackupController) isResponsibleFor(systemBackup *longhorn.SystemBackup) bool {
	return isControllerResponsibleFor(c.controllerID, c.ds, systemBackup.Name, "", systemBackup.Status.OwnerID)
}

func (c *SystemBackupController) syncSystemBackup(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync SystemBackup %v", c.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	if namespace != c.namespace {
		return nil
	}

	backupTarget, err := c.ds.GetDefaultBackupTargetRO()
	if err != nil {
		return err
	}

	backupTargetClient, err := newBackupTargetClientFromDefaultEngineImage(c.ds, backupTarget)
	if err != nil {
		return err
	}

	return c.reconcile(name, backupTargetClient)
}

func getLoggerForSystemBackup(logger logrus.FieldLogger, systemBackup *longhorn.SystemBackup) *logrus.Entry {
	return logger.WithField("systemBackup", systemBackup.Name)
}

func (c *SystemBackupController) LogErrorState(record *systemBackupRecord, systemBackup *longhorn.SystemBackup, log logrus.FieldLogger) {
	log.Error(record.message)
	c.eventRecorder.Eventf(systemBackup, corev1.EventTypeWarning, constant.EventReasonFailed, util.CapitalizeFirstLetter(record.message))
}

func (c *SystemBackupController) LogNormalState(record *systemBackupRecord, systemBackup *longhorn.SystemBackup, log logrus.FieldLogger) {
	log.Info(record.message)
	c.eventRecorder.Eventf(systemBackup, corev1.EventTypeNormal, record.reason, record.message)
}

func (c *SystemBackupController) recordErrorState(record *systemBackupRecord, systemBackup *longhorn.SystemBackup) {
	systemBackup.Status.State = longhorn.SystemBackupStateError
	systemBackup.Status.Conditions = types.SetCondition(
		systemBackup.Status.Conditions,
		longhorn.SystemBackupConditionTypeError,
		longhorn.ConditionStatusTrue,
		record.reason,
		record.message,
	)
}

func (c *SystemBackupController) recordNormalState(record *systemBackupRecord, systemBackup *longhorn.SystemBackup) {
	systemBackup.Status.State = record.nextState
}

func (c *SystemBackupController) updateSystemBackupRecord(record *systemBackupRecord, recordType systemBackupRecordType, nextState longhorn.SystemBackupState, reason, message string) {
	record.recordType = recordType
	record.nextState = nextState
	record.reason = reason
	record.message = message
}

func (c *SystemBackupController) reconcile(name string, backupTargetClient engineapi.SystemBackupOperationInterface) (err error) {
	systemBackup, err := c.ds.GetSystemBackup(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if !c.isResponsibleFor(systemBackup) {
		return nil
	}

	log := getLoggerForSystemBackup(c.logger, systemBackup)

	if systemBackup.Status.OwnerID != c.controllerID {
		systemBackup.Status.OwnerID = c.controllerID
		systemBackup, err = c.ds.UpdateSystemBackupStatus(systemBackup)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}

		log.Infof("Picked up by SystemBackup Controller %v", c.controllerID)
	}

	record := &systemBackupRecord{}
	existingSystemBackup := systemBackup.DeepCopy()
	defer c.handleStatusUpdate(record, systemBackup, existingSystemBackup, err, log)

	if !systemBackup.DeletionTimestamp.IsZero() &&
		systemBackup.Status.State != longhorn.SystemBackupStateDeleting {
		c.updateSystemBackupRecord(record,
			systemBackupRecordTypeNormal, longhorn.SystemBackupStateDeleting,
			constant.EventReasonDeleting, SystemBackupMsgDeletingRemote,
		)
		return
	}

	tempBackupArchivePath := filepath.Join(SystemBackupTempDir, systemBackup.Name+types.SystemBackupExtension)
	tempBackupDir := filepath.Join(SystemBackupTempDir, systemBackup.Name)

	switch systemBackup.Status.State {
	case longhorn.SystemBackupStateSyncing:
		err = syncSystemBackupFromBackupTarget(systemBackup, backupTargetClient)
		if err != nil {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeError, longhorn.SystemBackupStateError,
				longhorn.SystemBackupConditionReasonSync, SystemBackupErrSync,
			)
			return err
		}

		c.updateSystemBackupRecord(record,
			systemBackupRecordTypeNormal, longhorn.SystemBackupStateReady,
			constant.EventReasonSynced, SystemBackupMsgSyncedBackupTarget,
		)

	case longhorn.SystemBackupStateNone:
		// Sync the ready SystemBackups that are not in the current cluster.
		if isSystemBackupFromRemoteBackupTarget(systemBackup) {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeNormal, longhorn.SystemBackupStateSyncing,
				constant.EventReasonSyncing, SystemBackupMsgSyncingBackupTarget,
			)

			return
		}

		var longhornVersion string
		longhornVersion, err = getSystemBackupVersionExistInRemoteBackupTarget(systemBackup, backupTargetClient)
		if err != nil {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeError, longhorn.SystemBackupStateError,
				constant.EventReasonStart, err.Error(),
			)
			return nil
		}

		if longhornVersion != "" {
			if err := datastore.LabelSystemBackupVersion(longhornVersion, systemBackup); err != nil {
				return err
			}

			_, err = c.ds.UpdateSystemBackup(systemBackup)
			if err != nil {
				return err
			}
			return
		}

		err = c.InitSystemBackup(systemBackup, log)
		if err != nil {
			return err
		}

		c.updateSystemBackupRecord(record,
			systemBackupRecordTypeNormal, longhorn.SystemBackupStateGenerating,
			constant.EventReasonStart, SystemBackupMsgStarting,
		)

	case longhorn.SystemBackupStateGenerating:
		go c.GenerateSystemBackup(systemBackup, tempBackupArchivePath, tempBackupDir)

	case longhorn.SystemBackupStateUploading:
		go c.UploadSystemBackup(systemBackup, tempBackupArchivePath, tempBackupDir, backupTargetClient)

	case longhorn.SystemBackupStateReady, longhorn.SystemBackupStateError:
		cleanupLocalSystemBackupFiles(tempBackupArchivePath, tempBackupDir, log)

	case longhorn.SystemBackupStateDeleting:
		cleanupRemoteSystemBackupFiles(systemBackup, backupTargetClient, log)

		cleanupLocalSystemBackupFiles(tempBackupArchivePath, tempBackupDir, log)

		err = c.ds.RemoveFinalizerForSystemBackup(systemBackup)
		if err != nil {
			return err
		}
	}

	return nil
}

func getSystemBackupVersionExistInRemoteBackupTarget(systemBackup *longhorn.SystemBackup, backupTargetClient engineapi.SystemBackupOperationInterface) (string, error) {
	systemBackupsInBackupstore, err := backupTargetClient.ListSystemBackup()
	if err != nil {
		return "", errors.Wrap(err, "failed to list system backups in backup target")
	}

	for name, uri := range systemBackupsInBackupstore {
		if string(name) != systemBackup.Name {
			continue
		}

		longhornVersion, _, err := parseSystemBackupURI(string(uri))
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse system backup URI: %v", uri)
		}

		return longhornVersion, nil
	}
	return "", nil
}

func isSystemBackupFromRemoteBackupTarget(systemBackup *longhorn.SystemBackup) bool {
	return systemBackup.Labels != nil && systemBackup.Labels[types.GetVersionLabelKey()] != ""
}

func syncSystemBackupFromBackupTarget(systemBackup *longhorn.SystemBackup, backupTargetClient engineapi.SystemBackupOperationInterface) error {
	if systemBackup.Labels == nil {
		err := fmt.Errorf("missing %v label", types.GetVersionLabelKey())
		return errors.Wrapf(err, SystemBackupErrSync)
	}

	longhornVersion := systemBackup.Labels[types.GetVersionLabelKey()]
	if longhornVersion == "" {
		err := fmt.Errorf("missing %v label value", types.GetVersionLabelKey())
		return errors.Wrapf(err, SystemBackupErrSync)
	}

	systemBackupCfg, err := backupTargetClient.GetSystemBackupConfig(systemBackup.Name, longhornVersion)
	if err != nil {
		return err
	}

	systemBackup.Status.Version = systemBackupCfg.LonghornVersion
	systemBackup.Status.GitCommit = systemBackupCfg.LonghornGitCommit
	systemBackup.Status.ManagerImage = systemBackupCfg.ManagerImage
	systemBackup.Status.CreatedAt = metav1.Time{Time: systemBackupCfg.CreatedAt}
	return nil
}

func (c *SystemBackupController) InitSystemBackup(systemBackup *longhorn.SystemBackup, log logrus.FieldLogger) error {
	systemBackup.Status.Version = meta.Version
	systemBackup.Status.GitCommit = meta.GitCommit
	return nil
}

func (c *SystemBackupController) UploadSystemBackup(systemBackup *longhorn.SystemBackup, archievePath, tempDir string, backupTargetClient engineapi.SystemBackupOperationInterface) {
	log := getLoggerForSystemBackup(c.logger, systemBackup)

	var recordErr error
	existingSystemBackup := systemBackup.DeepCopy()
	// Handle the CR status update here because this method is called by a separate goroutine.
	defer func() {
		record := &systemBackupRecord{}
		if recordErr != nil {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeError, longhorn.SystemBackupStateError,
				longhorn.SystemBackupConditionReasonUpload, recordErr.Error(),
			)
		} else {
			systemBackup.Status.ManagerImage = c.managerImage
			systemBackup.Status.CreatedAt = metav1.Time{Time: time.Now().UTC()}

			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeNormal, longhorn.SystemBackupStateReady,
				longhorn.SystemBackupConditionReasonUpload, SystemBackupMsgUploadBackupTarget,
			)
		}

		c.handleStatusUpdate(record, systemBackup, existingSystemBackup, recordErr, log)
	}()

	defaultEngineImage, err := c.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		recordErr = errors.Wrapf(err, SystemBackupErrGetFmt, "default engine image")
		return
	}

	timeout := time.Duration(datastore.SystemBackupTimeout) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var systemBackupCfg *systembackupstore.Config
	for {
		select {
		case <-timer.C:
			recordErr = errors.Wrap(err, SystemBackupErrTimeoutUpload)
			return
		case <-ticker.C:
			systemBackupCfg, err = backupTargetClient.GetSystemBackupConfig(systemBackup.Name, systemBackup.Status.Version)
			if err != nil && !types.ErrorIsNotFound(err) {
				log.WithError(err).Debugf("Getting system backup config")
				err = errors.Wrap(err, SystemBackupErrGetConfig)
				continue
			}

			if systemBackupCfg != nil {
				return
			}

			_, err = backupTargetClient.UploadSystemBackup(systemBackup.Name, archievePath, systemBackup.Status.Version, systemBackup.Status.GitCommit, c.managerImage, defaultEngineImage)
			if err != nil {
				if types.ErrorAlreadyExists(err) {
					err = nil
				}

				log.WithError(err).Debugf("Failed to upload %v to backup target", archievePath)
			}
		}
	}
}

func cleanupRemoteSystemBackupFiles(systemBackup *longhorn.SystemBackup, backupTargetClient engineapi.SystemBackupOperationInterface, log logrus.FieldLogger) {
	if systemBackup.Status.Version == "" {
		// The backup store sync might not have finished
		return
	}

	systemBackupsFromBackupTarget, err := backupTargetClient.ListSystemBackup()
	if err != nil {
		log.WithError(err).Warn("Failed to list system backups in backup target")
		return
	}

	if _, exist := systemBackupsFromBackupTarget[systembackupstore.Name(systemBackup.Name)]; !exist {
		return
	}

	_, err = backupTargetClient.DeleteSystemBackup(systemBackup)
	if err != nil && !types.ErrorIsNotFound(err) {
		log.WithError(err).Warnf("Failed to delete %v system backup in backup target", systemBackup.Name)
		return
	}

	systemBackupCfg, err := backupTargetClient.GetSystemBackupConfig(systemBackup.Name, systemBackup.Status.Version)
	if err != nil && !types.ErrorIsNotFound(err) {
		log.WithError(err).Warn(SystemBackupErrGetConfig)
		return
	}

	if systemBackupCfg != nil {
		log.Warn("Failed to check the system backup deletion, because it's being deleted")
		return
	}

	log.Info("Deleted system backup in backup target")
}

type systemBackupMeta struct {
	LonghornVersion       string      `json:"longhornVersion"`
	LonghornGitCommit     string      `json:"longhornGitCommit"`
	KubernetesVersion     string      `json:"kubernetesVersion"`
	LonghornNamespaceUUID string      `json:"longhornNamspaceUUID"`
	SystemBackupCreatedAt metav1.Time `json:"systemBackupCreatedAt"`
	ManagerImage          string      `json:"managerImage"`
}

func (c *SystemBackupController) newSystemBackupMeta(systemBackup *longhorn.SystemBackup) (*systemBackupMeta, error) {
	namespace, err := c.ds.GetNamespace(systemBackup.Namespace)
	if err != nil {
		return nil, err
	}

	kubeVersion, err := c.ds.GetKubernetesVersion()
	if err != nil {
		return nil, err
	}

	return &systemBackupMeta{
		LonghornVersion:       meta.Version,
		LonghornGitCommit:     meta.GitCommit,
		KubernetesVersion:     kubeVersion.GitVersion,
		LonghornNamespaceUUID: string(namespace.UID),
		SystemBackupCreatedAt: systemBackup.Status.CreatedAt,
		ManagerImage:          c.managerImage,
	}, nil
}

func (c *SystemBackupController) GenerateSystemBackup(systemBackup *longhorn.SystemBackup, archievePath, tempDir string) {
	log := getLoggerForSystemBackup(c.logger, systemBackup)

	var err error
	var errMessage string
	existingSystemBackup := systemBackup.DeepCopy()
	defer func() {
		record := &systemBackupRecord{}
		if err != nil {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeError, longhorn.SystemBackupStateError,
				longhorn.SystemBackupConditionReasonGenerate, errMessage,
			)
		} else {
			c.updateSystemBackupRecord(record,
				systemBackupRecordTypeNormal, longhorn.SystemBackupStateUploading,
				longhorn.SystemBackupConditionReasonGenerate, fmt.Sprintf(SystemBackupMsgCreatedArchieveFmt, archievePath),
			)
		}

		c.handleStatusUpdate(record, systemBackup, existingSystemBackup, err, log)
	}()

	systemBackupMeta, err := c.newSystemBackupMeta(systemBackup)
	if err != nil {
		errMessage = fmt.Sprint(errors.Wrapf(err, SystemBackupErrGetFmt, "system backup meta"))
	}

	err = os.MkdirAll(tempDir, os.FileMode(0755))
	if err != nil {
		errMessage = fmt.Sprint(errors.Wrap(err, SystemBackupErrMkdir))
		return
	}

	err = c.generateSystemBackup(systemBackupMeta, tempDir)
	if err != nil {
		errMessage = fmt.Sprint(errors.Wrap(err, SystemBackupErrGenerate))
		return
	}

	cmd := exec.Command("zip", "-r", filepath.Base(archievePath), systemBackup.Name)
	cmd.Dir = filepath.Dir(tempDir)
	err = cmd.Run()
	if err != nil {
		errMessage = fmt.Sprint(errors.Wrap(err, SystemBackupErrArchive))
		return
	}

	_, err = os.Stat(archievePath)
	if err != nil {
		errMessage = fmt.Sprint(errors.Wrap(err, SystemBackupErrOSStat))
		return
	}
}

func (c *SystemBackupController) generateSystemBackup(systemBackupMeta *systemBackupMeta, tempDir string) (err error) {
	metaFile := filepath.Join(tempDir, "metadata.yaml")
	err = util.EncodeToYAMLFile(systemBackupMeta, metaFile)
	if err != nil {
		return err
	}

	return c.generateSystemBackupYAMLs(filepath.Join(tempDir, "yamls"))
}

func (c *SystemBackupController) generateSystemBackupYAMLs(yamlsDir string) (err error) {
	defer func() {
		err = errors.Wrap(err, SystemBackupErrGenerateYAML)
	}()

	schemeGenerateFns := map[string]func(string) error{
		filepath.Join(yamlsDir, "apiextensions"): c.generateSystemBackupYAMLsForAPIExtensions,
		filepath.Join(yamlsDir, "kubernetes"):    c.generateSystemBackupYAMLsForKubernetes,
		filepath.Join(yamlsDir, "longhorn"):      c.generateSystemBackupYAMLsForLonghorn,
	}
	for scheme, fn := range schemeGenerateFns {
		err = fn(scheme)
		if err != nil {
			return
		}
	}
	return
}

func (c *SystemBackupController) generateSystemBackupYAMLsForLonghorn(dir string) (err error) {
	scheme := runtime.NewScheme()
	err = longhorn.AddToScheme(scheme)
	if err != nil {
		return errors.Wrap(err, "failed to add Longhorn to scheme")
	}

	// TODO: handle BackingImage in https://github.com/longhorn/longhorn/issues/4165
	resourceGetFns := map[string]func() (runtime.Object, error){
		"setting":       c.ds.GetAllLonghornSettings,
		"engineimages":  c.ds.GetAllLonghornEngineImages,
		"volumes":       c.ds.GetAllLonghornVolumes,
		"recurringjobs": c.ds.GetAllLonghornRecurringJobs,
	}

	for name, fn := range resourceGetFns {
		err = getObjectsAndPrintToYAML(dir, name, fn, scheme)
		if err != nil {
			return
		}
	}

	return nil
}

func (c *SystemBackupController) generateSystemBackupYAMLsForKubernetes(dir string) (err error) {
	scheme := kubernetesscheme.Scheme

	err = c.generateSystemBackupYAMLsForServiceAccount(dir, "serviceaccounts", "clusterroles", "clusterrolebindings", scheme)
	if err != nil {
		return
	}

	err = c.generateSystemBackupYAMLsForPodSecurityPolicy(dir, "roles", "rolebindings", "podsecuritypolicies", scheme)
	if err != nil {
		return
	}

	err = getObjectsAndPrintToYAML(dir, "daemonsets", c.ds.GetAllDaemonSetsList, scheme)
	if err != nil {
		return
	}

	err = getObjectsAndPrintToYAML(dir, "deployments", c.ds.GetAllDeploymentsList, scheme)
	if err != nil {
		return
	}

	err = getObjectsAndPrintToYAML(dir, "configmaps", c.ds.GetAllConfigMaps, scheme)
	if err != nil {
		return
	}

	err = c.generateSystemBackupYAMLsForServices(dir, "services", scheme)
	if err != nil {
		return
	}

	err = getObjectsAndPrintToYAML(dir, "storageclasses", c.ds.GetAllLonghornStorageClassList, scheme)
	if err != nil {
		return
	}

	err = getObjectsAndPrintToYAML(dir, "persistentvolumes", c.ds.GetAllPersistentVolumesWithLonghornProvisioner, scheme)
	if err != nil {
		return
	}

	return getObjectsAndPrintToYAML(dir, "persistentvolumeclaims", c.ds.GetAllPersistentVolumeClaimsByPersistentVolumeProvisioner, scheme)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForServices(dir, name string, scheme *runtime.Scheme) (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to generate Longhorn Services")
	}()

	obj, err := c.ds.GetAllServicesList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all Longhorn services")

	}

	serviceList, ok := obj.(*corev1.ServiceList)
	if !ok {
		return errors.Wrap(err, "failed to convert to serviceList object")
	}

	services := []corev1.Service{}
	for _, service := range serviceList.Items {
		if service.Spec.ClusterIP != "" {
			service.Spec.ClusterIP = ""
		}

		if service.Spec.ClusterIPs != nil {
			service.Spec.ClusterIPs = nil
		}

		services = append(services, service)
	}

	serviceList.Items = services

	return getObjectsAndPrintToYAML(dir, name, func() (runtime.Object, error) {
		return serviceList, nil
	}, scheme)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForPodSecurityPolicy(dir,
	roleName, roleBindingName, podSecurityPolicyName string,
	scheme *runtime.Scheme) (err error) {
	// Generate Role YAML
	roleObj, err := c.ds.GetAllRoleList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all Longhorn roles")
	}

	roleList, ok := roleObj.(*rbacv1.RoleList)
	if !ok {
		return errors.Wrap(err, "failed to convert to roleList object")
	}

	err = getObjectsAndPrintToYAML(dir, roleName, func() (runtime.Object, error) {
		return roleList, nil
	}, scheme)
	if err != nil {
		return
	}

	// Generate PodSecurityPolicy YAML
	err = c.generateSystemBackupYAMLsForPodSecurityPoliciesByRoles(roleList, dir, podSecurityPolicyName, scheme)
	if err != nil {
		return
	}

	// Generate RoleBinding YAML
	return getObjectsAndPrintToYAML(dir, roleBindingName, c.ds.GetAllRoleBindingList, scheme)

}

func (c *SystemBackupController) generateSystemBackupYAMLsForPodSecurityPoliciesByRoles(
	roleList *rbacv1.RoleList,
	dir, name string, scheme *runtime.Scheme) (err error) {
	pspObj, err := c.ds.GetAllPodSecurityPolicyList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all podSecurityPolicies")
	}

	pspList, ok := pspObj.(*policyv1beta1.PodSecurityPolicyList)
	if !ok {
		return errors.Wrap(err, "failed to convert to podSecurityPolicyList object")
	}

	filtered := []policyv1beta1.PodSecurityPolicy{}
	for _, psp := range pspList.Items {
		shouldBackup := false
		for _, role := range roleList.Items {
			for _, rule := range role.Rules {
				if util.Contains(rule.ResourceNames, psp.Name) {
					shouldBackup = true
					break
				}
			}

			if shouldBackup {
				break
			}
		}

		if shouldBackup {
			filtered = append(filtered, psp)
		}
	}
	pspList.Items = filtered

	return getObjectsAndPrintToYAML(dir, name, func() (runtime.Object, error) {
		return pspList, nil
	}, scheme)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForServiceAccount(dir,
	serviceAccountName, clusterRoleName, clusterRoleBindingName string,
	scheme *runtime.Scheme) (err error) {
	serviceAccountObj, err := c.ds.GetAllServiceAccountList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all Longhorn ServiceAccounts")

	}

	serviceAccountList, ok := serviceAccountObj.(*corev1.ServiceAccountList)
	if !ok {
		return errors.Wrap(err, "failed to convert to ServiceAccountList object")
	}

	err = getObjectsAndPrintToYAML(dir, serviceAccountName, func() (runtime.Object, error) {
		return serviceAccountList, nil
	}, scheme)
	if err != nil {
		return
	}

	// Generate ClusterRoleBinding from Longhorn ServieAccount
	clusterRoleBindingObj, err := c.ds.GetAllClusterRoleBindingList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all ClusterRoleBindings")

	}

	clusterRoleBindingList, ok := clusterRoleBindingObj.(*rbacv1.ClusterRoleBindingList)
	if !ok {
		return errors.Wrap(err, "failed to convert to ClusterRoleBindingList object")
	}

	err = c.generateSystemBackupYAMLsForClusterRoleBindingsByServiceAccounts(
		clusterRoleBindingList, serviceAccountList, dir, clusterRoleName, scheme,
	)
	if err != nil {
		return err
	}

	// Generate ClusterRole YAML from ClusterRoleBinding
	clusterRoleObj, err := c.ds.GetAllClusterRoleList()
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get all ClusterRoles")

	}

	clusterRoleList, ok := clusterRoleObj.(*rbacv1.ClusterRoleList)
	if !ok {
		return errors.Wrap(err, "failed to convert to ClusterRoleList object")
	}

	return c.generateSystemBackupYAMLsForClusterRolesByClusterRoleBindings(
		clusterRoleList, clusterRoleBindingList, dir, clusterRoleBindingName, scheme,
	)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForClusterRolesByClusterRoleBindings(
	clusterRoleList *rbacv1.ClusterRoleList,
	clusterRoleBindingList *rbacv1.ClusterRoleBindingList,
	dir, name string, scheme *runtime.Scheme) (err error) {
	RoleRefNames := map[string]struct{}{}
	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		if _, exist := RoleRefNames[clusterRoleBinding.RoleRef.Name]; exist {
			continue
		}
		RoleRefNames[clusterRoleBinding.RoleRef.Name] = struct{}{}
	}

	filtered := []rbacv1.ClusterRole{}
	for _, clusterRole := range clusterRoleList.Items {
		if _, exist := RoleRefNames[clusterRole.Name]; !exist {
			continue
		}
		filtered = append(filtered, clusterRole)

	}
	clusterRoleList.Items = filtered

	return getObjectsAndPrintToYAML(dir, name, func() (runtime.Object, error) {
		return clusterRoleList, nil
	}, scheme)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForClusterRoleBindingsByServiceAccounts(
	clusterRoleBindingList *rbacv1.ClusterRoleBindingList,
	serviceAccountList *corev1.ServiceAccountList,
	dir, name string, scheme *runtime.Scheme) (err error) {
	filtered := []rbacv1.ClusterRoleBinding{}
	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		shouldBackup := false
		for _, serviceAccount := range serviceAccountList.Items {
			for _, subject := range clusterRoleBinding.Subjects {
				if subject.Kind != types.KubernetesKindServiceAccount {
					continue
				}
				if subject.Name != serviceAccount.Name {
					continue
				}
				if subject.Namespace != serviceAccount.Namespace {
					continue
				}
				shouldBackup = true
				break
			}
			if shouldBackup {
				break
			}
		}
		if shouldBackup {
			filtered = append(filtered, clusterRoleBinding)
		}
	}
	clusterRoleBindingList.Items = filtered

	return getObjectsAndPrintToYAML(dir, name, func() (runtime.Object, error) {
		return clusterRoleBindingList, nil
	}, scheme)
}

func (c *SystemBackupController) generateSystemBackupYAMLsForAPIExtensions(dir string) (err error) {
	scheme := runtime.NewScheme()
	err = apiextensionsv1.AddToScheme(scheme)
	if err != nil {
		return errors.Wrap(err, "failed to add API Extension to scheme")
	}

	return getObjectsAndPrintToYAML(dir, "customresourcedefinitions", c.ds.GetAllLonghornCustomResourceDefinitions, scheme)
}

type GetRuntimeObjectListFunc func() (runtime.Object, error)

func getObjectsAndPrintToYAML(dir, name string, getListFunc GetRuntimeObjectListFunc, scheme *runtime.Scheme) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to generate %v", name)
	}()

	obj, err := getListFunc()
	if err != nil {
		return
	}

	err = addTypeInformationToObject(obj, scheme)
	if err != nil {
		return
	}

	err = os.MkdirAll(dir, os.FileMode(0755))
	if err != nil {
		return
	}

	path := filepath.Join(dir, name+".yaml")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	printer := printers.YAMLPrinter{}
	err = printer.PrintObj(obj, f)
	if err != nil {
		return
	}

	return nil
}

func addTypeInformationToObject(obj runtime.Object, scheme *runtime.Scheme) error {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return errors.Wrap(err, "failed to set ObjectKind, could missing apiVersion or kind and cannot assign it")
	}

	for _, gvk := range gvks {
		if len(gvk.Kind) == 0 {
			continue
		}
		if len(gvk.Version) == 0 || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		break
	}

	return nil
}

func cleanupLocalSystemBackupFiles(archievePath, tempDir string, log logrus.FieldLogger) {
	if err := os.Remove(archievePath); err != nil && !os.IsNotExist(err) {
		log.WithError(err).Warn(SystemBackupErrRemove)
	}

	if err := os.RemoveAll(tempDir); err != nil && !os.IsNotExist(err) {
		log.WithError(err).Warn(SystemBackupErrRemoveAll)
	}
}

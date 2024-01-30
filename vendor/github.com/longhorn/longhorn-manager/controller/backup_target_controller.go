package controller

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	systembackupstore "github.com/longhorn/backupstore/systembackup"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type BackupTargetController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	proxyConnCounter util.Counter
}

func NewBackupTargetController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
	proxyConnCounter util.Counter) *BackupTargetController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	btc := &BackupTargetController{
		baseController: newBaseController("longhorn-backup-target", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-backup-target-controller"}),

		proxyConnCounter: proxyConnCounter,
	}

	ds.BackupTargetInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    btc.enqueueBackupTarget,
		UpdateFunc: func(old, cur interface{}) { btc.enqueueBackupTarget(cur) },
	})
	btc.cacheSyncs = append(btc.cacheSyncs, ds.BackupTargetInformer.HasSynced)

	ds.EngineImageInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) {
			oldEI := old.(*longhorn.EngineImage)
			curEI := cur.(*longhorn.EngineImage)
			if curEI.ResourceVersion == oldEI.ResourceVersion {
				// Periodic resync will send update events for all known secrets.
				// Two different versions of the same secret will always have different RVs.
				// Ref to https://github.com/kubernetes/kubernetes/blob/c8ebc8ab75a9c36453cf6fa30990fd0a277d856d/pkg/controller/deployment/deployment_controller.go#L256-L263
				return
			}
			btc.enqueueEngineImage(cur)
		},
	}, 0)
	btc.cacheSyncs = append(btc.cacheSyncs, ds.EngineImageInformer.HasSynced)

	return btc
}

func (btc *BackupTargetController) enqueueBackupTarget(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	btc.queue.Add(key)
}

func (btc *BackupTargetController) enqueueEngineImage(obj interface{}) {
	ei, ok := obj.(*longhorn.EngineImage)
	if !ok {
		return
	}

	defaultEngineImage, err := btc.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	// Enqueue the backup target only when the default engine image becomes ready
	if err != nil || ei.Spec.Image != defaultEngineImage || ei.Status.State != longhorn.EngineImageStateDeployed {
		return
	}
	// For now, we only support a default backup target
	// We've to enhance it once we support multiple backup targets
	// https://github.com/longhorn/longhorn/issues/2317
	btc.queue.Add(ei.Namespace + "/" + types.DefaultBackupTargetName)
}

func (btc *BackupTargetController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer btc.queue.ShutDown()

	btc.logger.Info("Starting Longhorn Backup Target controller")
	defer btc.logger.Info("Shut down Longhorn Backup Target controller")

	if !cache.WaitForNamedCacheSync(btc.name, stopCh, btc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(btc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (btc *BackupTargetController) worker() {
	for btc.processNextWorkItem() {
	}
}

func (btc *BackupTargetController) processNextWorkItem() bool {
	key, quit := btc.queue.Get()
	if quit {
		return false
	}
	defer btc.queue.Done(key)
	err := btc.syncHandler(key.(string))
	btc.handleErr(err, key)
	return true
}

func (btc *BackupTargetController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync %v", btc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != btc.namespace {
		// Not ours, skip it
		return nil
	}
	if name != types.DefaultBackupTargetName {
		// For now, we only support a default backup target
		// We've to enhance it once we support multiple backup targets
		// https://github.com/longhorn/longhorn/issues/2317
		return nil
	}
	return btc.reconcile(name)
}

func (btc *BackupTargetController) handleErr(err error, key interface{}) {
	if err == nil {
		btc.queue.Forget(key)
		return
	}

	log := btc.logger.WithField("BackupTarget", key)
	if btc.queue.NumRequeues(key) < maxRetries {
		handleReconcileErrorLogging(log, err, "Failed to sync Longhorn backup target")
		btc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	handleReconcileErrorLogging(log, err, "Dropping Longhorn backup target out of the queue")
	btc.queue.Forget(key)
}

func getLoggerForBackupTarget(logger logrus.FieldLogger, backupTarget *longhorn.BackupTarget) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"url":      backupTarget.Spec.BackupTargetURL,
			"cred":     backupTarget.Spec.CredentialSecret,
			"interval": backupTarget.Spec.PollInterval.Duration,
		},
	)
}

func getBackupTarget(controllerID string, backupTarget *longhorn.BackupTarget, ds *datastore.DataStore, log logrus.FieldLogger, proxyConnCounter util.Counter) (engineClientProxy engineapi.EngineClientProxy, backupTargetClient *engineapi.BackupTargetClient, err error) {
	instanceManager, err := ds.GetDefaultInstanceManagerByNode(controllerID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get default engine instance manager for proxy client")
	}

	engineClientProxy, err = engineapi.NewEngineClientProxy(instanceManager, log, proxyConnCounter)
	if err != nil {
		return nil, nil, err
	}

	backupTargetClient, err = newBackupTargetClientFromDefaultEngineImage(ds, backupTarget)
	if err != nil {
		engineClientProxy.Close()
		return nil, nil, err
	}

	return engineClientProxy, backupTargetClient, nil
}

func newBackupTargetClient(ds *datastore.DataStore, backupTarget *longhorn.BackupTarget, engineImage string) (backupTargetClient *engineapi.BackupTargetClient, err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to get %v backup target client on %v", backupTarget.Name, engineImage)
	}()

	backupType, err := util.CheckBackupType(backupTarget.Spec.BackupTargetURL)
	if err != nil {
		return nil, err
	}

	var credential map[string]string
	if types.BackupStoreRequireCredential(backupType) {
		if backupTarget.Spec.CredentialSecret == "" {
			return nil, fmt.Errorf("could not access %s without credential secret", backupType)
		}
		credential, err = ds.GetCredentialFromSecret(backupTarget.Spec.CredentialSecret)
		if err != nil {
			return nil, err
		}
	}
	return engineapi.NewBackupTargetClient(engineImage, backupTarget.Spec.BackupTargetURL, credential), nil
}

func newBackupTargetClientFromDefaultEngineImage(ds *datastore.DataStore, backupTarget *longhorn.BackupTarget) (*engineapi.BackupTargetClient, error) {
	defaultEngineImage, err := ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return nil, err
	}

	return newBackupTargetClient(ds, backupTarget, defaultEngineImage)
}

func (btc *BackupTargetController) reconcile(name string) (err error) {
	backupTarget, err := btc.ds.GetBackupTarget(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	log := getLoggerForBackupTarget(btc.logger, backupTarget)

	// Every controller should do the clean up even it is not responsible for the CR
	if backupTarget.Spec.BackupTargetURL == "" {
		if err := btc.cleanUpAllMounts(backupTarget); err != nil {
			log.WithError(err).Warn("Failed to clean up all mount points")
		}
	}

	// Check the responsible node
	defaultEngineImage, err := btc.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return err
	}
	isResponsible, err := btc.isResponsibleFor(backupTarget, defaultEngineImage)
	if err != nil {
		return nil
	}
	if !isResponsible {
		return nil
	}
	if backupTarget.Status.OwnerID != btc.controllerID {
		backupTarget.Status.OwnerID = btc.controllerID
		backupTarget, err = btc.ds.UpdateBackupTargetStatus(backupTarget)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
	}

	// Check the controller should run synchronization
	if !backupTarget.Status.LastSyncedAt.IsZero() &&
		!backupTarget.Spec.SyncRequestedAt.After(backupTarget.Status.LastSyncedAt.Time) {
		return nil
	}

	var backupTargetClient *engineapi.BackupTargetClient
	existingBackupTarget := backupTarget.DeepCopy()
	syncTime := metav1.Time{Time: time.Now().UTC()}
	defer func() {
		if err != nil {
			return
		}
		if backupTargetClient != nil {
			// If there is something wrong with the backup target config and Longhorn cannot launch the client,
			// lacking the credential for example, Longhorn won't even try to connect with the remote backupstore.
			// In this case, the controller should not update `Status.LastSyncedAt`.
			backupTarget.Status.LastSyncedAt = syncTime
		}
		if reflect.DeepEqual(existingBackupTarget.Status, backupTarget.Status) {
			return
		}
		if _, err := btc.ds.UpdateBackupTargetStatus(backupTarget); err != nil && apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", name)
			btc.enqueueBackupTarget(backupTarget)
		}
	}()

	if backupTarget.Spec.BackupTargetURL == "" {
		backupTarget.Status.Available = false
		backupTarget.Status.Conditions = types.SetCondition(backupTarget.Status.Conditions,
			longhorn.BackupTargetConditionTypeUnavailable, longhorn.ConditionStatusTrue,
			longhorn.BackupTargetConditionReasonUnavailable, "backup target URL is empty")
		if err := btc.cleanupBackupVolumes(); err != nil {
			return errors.Wrap(err, "failed to clean up BackupVolumes")
		}

		if err := btc.cleanupSystemBackups(); err != nil {
			return errors.Wrap(err, "failed to clean up SystemBackups")
		}

		return nil
	}

	// Initialize a backup target client
	engineClientProxy, backupTargetClient, err := getBackupTarget(btc.controllerID, backupTarget, btc.ds, log, btc.proxyConnCounter)
	if err != nil {
		backupTarget.Status.Available = false
		backupTarget.Status.Conditions = types.SetCondition(backupTarget.Status.Conditions,
			longhorn.BackupTargetConditionTypeUnavailable, longhorn.ConditionStatusTrue,
			longhorn.BackupTargetConditionReasonUnavailable, err.Error())
		log.WithError(err).Error("Failed to init backup target clients")
		return nil // Ignore error to allow status update as well as preventing enqueue
	}
	defer engineClientProxy.Close()

	if err = btc.syncBackupVolume(backupTarget, backupTargetClient, syncTime, log); err != nil {
		return err
	}

	if !backupTarget.Status.Available {
		return nil
	}

	if err = btc.syncSystemBackup(backupTargetClient, log); err != nil {
		return err
	}
	return nil
}

func (btc *BackupTargetController) cleanUpAllMounts(backupTarget *longhorn.BackupTarget) (err error) {
	log := getLoggerForBackupTarget(btc.logger, backupTarget)
	engineClientProxy, backupTargetClient, err := getBackupTarget(btc.controllerID, backupTarget, btc.ds, log, btc.proxyConnCounter)
	if err != nil {
		return err
	}
	defer engineClientProxy.Close()
	err = backupTargetClient.BackupCleanUpAllMounts()
	return err
}

func (btc *BackupTargetController) syncBackupVolume(backupTarget *longhorn.BackupTarget, backupTargetClient *engineapi.BackupTargetClient, syncTime metav1.Time, log logrus.FieldLogger) error {
	// Get a list of all the backup volumes that are stored in the backup target
	res, err := backupTargetClient.BackupVolumeNameList(backupTargetClient.URL, backupTargetClient.Credential)
	if err != nil {
		backupTarget.Status.Available = false
		backupTarget.Status.Conditions = types.SetCondition(backupTarget.Status.Conditions,
			longhorn.BackupTargetConditionTypeUnavailable, longhorn.ConditionStatusTrue,
			longhorn.BackupTargetConditionReasonUnavailable, err.Error())
		log.WithError(err).Error("Failed to list backup volumes from backup target")
		return nil // Ignore error to allow status update as well as preventing enqueue
	}
	backupStoreBackupVolumes := sets.NewString(res...)

	// Get a list of all the backup volumes that exist as custom resources in the cluster
	clusterBackupVolumes, err := btc.ds.ListBackupVolumes()
	if err != nil {
		log.WithError(err).Error("Failed to list backup volumes in the cluster")
		return err
	}

	clusterBackupVolumesSet := sets.NewString()
	for _, b := range clusterBackupVolumes {
		clusterBackupVolumesSet.Insert(b.Name)
	}

	// TODO: add a unit test, separate to a function
	// Get a list of backup volumes that *are* in the backup target and *aren't* in the cluster
	// and create the BackupVolume CR in the cluster
	backupVolumesToPull := backupStoreBackupVolumes.Difference(clusterBackupVolumesSet)
	if count := backupVolumesToPull.Len(); count > 0 {
		log.Infof("Found %d backup volumes in the backup target that do not exist in the cluster and need to be pulled", count)
	}
	for backupVolumeName := range backupVolumesToPull {
		backupVolume := &longhorn.BackupVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: backupVolumeName,
			},
		}
		if _, err = btc.ds.CreateBackupVolume(backupVolume); err != nil && !apierrors.IsAlreadyExists(err) {
			log.WithError(err).Errorf("Failed to create backup volume %s in the cluster", backupVolumeName)
			return err
		}
	}

	// TODO: add a unit test, separate to a function
	// Get a list of backup volumes that *are* in the cluster and *aren't* in the backup target
	// and delete the BackupVolume CR in the cluster
	backupVolumesToDelete := clusterBackupVolumesSet.Difference(backupStoreBackupVolumes)
	if count := backupVolumesToDelete.Len(); count > 0 {
		log.Infof("Found %d backup volumes in the backup target that do not exist in the backup target and need to be deleted", count)
	}
	for backupVolumeName := range backupVolumesToDelete {
		log.WithField("backupVolume", backupVolumeName).Info("Deleting backup volume from cluster")
		if err = btc.ds.DeleteBackupVolume(backupVolumeName); err != nil {
			log.WithError(err).Errorf("Failed to delete backup volume %s from cluster", backupVolumeName)
			return err
		}
	}

	// Update the BackupVolume CR spec.syncRequestAt to request the
	// backup_volume_controller to reconcile the BackupVolume CR
	for backupVolumeName, backupVolume := range clusterBackupVolumes {
		backupVolume.Spec.SyncRequestedAt = syncTime
		if _, err = btc.ds.UpdateBackupVolume(backupVolume); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Errorf("Failed to update backup volume %s spec", backupVolumeName)
		}
	}

	// Update the backup target status
	backupTarget.Status.Available = true
	backupTarget.Status.Conditions = types.SetCondition(backupTarget.Status.Conditions,
		longhorn.BackupTargetConditionTypeUnavailable, longhorn.ConditionStatusFalse,
		"", "")
	return nil
}

func (btc *BackupTargetController) syncSystemBackup(backupTargetClient *engineapi.BackupTargetClient, log logrus.FieldLogger) error {
	systemBackupsFromBackupTarget, err := backupTargetClient.ListSystemBackup()
	if err != nil {
		return errors.Wrapf(err, "failed to list system backups in %v", backupTargetClient.URL)
	}

	clusterSystemBackups, err := btc.ds.ListSystemBackups()
	if err != nil {
		return errors.Wrap(err, "failed to list SystemBackups")
	}

	clusterReadySystemBackupNames := sets.NewString()
	for _, systemBackup := range clusterSystemBackups {
		if systemBackup.Status.State != longhorn.SystemBackupStateReady {
			continue
		}
		clusterReadySystemBackupNames.Insert(systemBackup.Name)
	}

	backupstoreSystemBackupNames := sets.NewString(util.GetSortedKeysFromMap(systemBackupsFromBackupTarget)...)

	// Create SystemBackup from the system backups in the backup store if not already exist in the cluster.
	addSystemBackupsToCluster := backupstoreSystemBackupNames.Difference(clusterReadySystemBackupNames)
	for name := range addSystemBackupsToCluster {
		systemBackupURI := systemBackupsFromBackupTarget[systembackupstore.Name(name)]
		longhornVersion, _, err := parseSystemBackupURI(string(systemBackupURI))
		if err != nil {
			return errors.Wrapf(err, "failed to parse system backup URI: %v", systemBackupURI)
		}

		log.WithField("systemBackup", name).Info("Creating SystemBackup from remote backup target")
		systemBackup := &longhorn.SystemBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					// Label with the version to be used by the system-backup controller
					// to get the config from the backup target.
					types.GetVersionLabelKey(): longhornVersion,
				},
			},
		}
		_, err = btc.ds.CreateSystemBackup(systemBackup)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create SystemBackup %v from remote backup target", name)
		}

	}

	// Delete ready SystemBackup that doesn't exist in the backup store.
	delSystemBackupsInCluster := clusterReadySystemBackupNames.Difference(backupstoreSystemBackupNames)
	for name := range delSystemBackupsInCluster {
		log.WithField("systemBackup", name).Info("Deleting SystemBackup not exist in backupstore")
		if err = btc.ds.DeleteSystemBackup(name); err != nil {
			return errors.Wrapf(err, "failed to delete SystemBackup %v not exist in backupstore", name)
		}
	}

	return nil
}

func (btc *BackupTargetController) isResponsibleFor(bt *longhorn.BackupTarget, defaultEngineImage string) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	isResponsible := isControllerResponsibleFor(btc.controllerID, btc.ds, bt.Name, "", bt.Status.OwnerID)

	currentOwnerEngineAvailable, err := btc.ds.CheckEngineImageReadiness(defaultEngineImage, bt.Status.OwnerID)
	if err != nil {
		return false, err
	}
	currentNodeEngineAvailable, err := btc.ds.CheckEngineImageReadiness(defaultEngineImage, btc.controllerID)
	if err != nil {
		return false, err
	}

	isPreferredOwner := currentNodeEngineAvailable && isResponsible
	continueToBeOwner := currentNodeEngineAvailable && btc.controllerID == bt.Status.OwnerID
	requiresNewOwner := currentNodeEngineAvailable && !currentOwnerEngineAvailable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

// cleanupBackupVolumes deletes all BackupVolume CRs
func (btc *BackupTargetController) cleanupBackupVolumes() error {
	clusterBackupVolumes, err := btc.ds.ListBackupVolumes()
	if err != nil {
		return err
	}

	var errs []string
	for backupVolumeName := range clusterBackupVolumes {
		if err = btc.ds.DeleteBackupVolume(backupVolumeName); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err.Error())
			continue
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ","))
	}
	return nil
}

// cleanupSystemBackups deletes all SystemBackup CRs
func (btc *BackupTargetController) cleanupSystemBackups() error {
	systemBackups, err := btc.ds.ListSystemBackups()
	if err != nil {
		return err
	}

	var errs []string
	for systemBackup := range systemBackups {
		if err = btc.ds.DeleteSystemBackup(systemBackup); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err.Error())
			continue
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ","))
	}
	return nil
}

// parseSystemBackupURI and return version and name.
// Ex: v1.4.0, sample-system-backup, nil = parseSystemBackupURI("backupstore/system-backups/v1.4.0/sample-system-backup")
func parseSystemBackupURI(uri string) (version, name string, err error) {
	split := strings.Split(uri, "/")
	if len(split) < 2 {
		return "", "", errors.Errorf("invalid system-backup URI: %v", uri)
	}

	return split[len(split)-2], split[len(split)-1], nil
}

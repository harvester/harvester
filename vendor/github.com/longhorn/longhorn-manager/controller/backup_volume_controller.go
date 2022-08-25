package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/backupstore"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type BackupVolumeController struct {
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

func NewBackupVolumeController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
	proxyConnCounter util.Counter,
) *BackupVolumeController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	bvc := &BackupVolumeController{
		baseController: newBaseController("longhorn-backup-volume", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-backup-volume-controller"}),

		proxyConnCounter: proxyConnCounter,
	}

	ds.BackupVolumeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    bvc.enqueueBackupVolume,
		UpdateFunc: func(old, cur interface{}) { bvc.enqueueBackupVolume(cur) },
		DeleteFunc: bvc.enqueueBackupVolume,
	})
	bvc.cacheSyncs = append(bvc.cacheSyncs, ds.BackupVolumeInformer.HasSynced)

	return bvc
}

func (bvc *BackupVolumeController) enqueueBackupVolume(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	bvc.queue.Add(key)
}

func (bvc *BackupVolumeController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer bvc.queue.ShutDown()

	bvc.logger.Infof("Start Longhorn Backup Volume controller")
	defer bvc.logger.Infof("Shutting down Longhorn Backup Volume controller")

	if !cache.WaitForNamedCacheSync(bvc.name, stopCh, bvc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(bvc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (bvc *BackupVolumeController) worker() {
	for bvc.processNextWorkItem() {
	}
}

func (bvc *BackupVolumeController) processNextWorkItem() bool {
	key, quit := bvc.queue.Get()
	if quit {
		return false
	}
	defer bvc.queue.Done(key)
	err := bvc.syncHandler(key.(string))
	bvc.handleErr(err, key)
	return true
}

func (bvc *BackupVolumeController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync backup volume %v", bvc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != bvc.namespace {
		// Not ours, skip it
		return nil
	}
	return bvc.reconcile(name)
}

func (bvc *BackupVolumeController) handleErr(err error, key interface{}) {
	if err == nil {
		bvc.queue.Forget(key)
		return
	}

	if bvc.queue.NumRequeues(key) < maxRetries {
		bvc.logger.WithError(err).Warnf("Error syncing Longhorn backup volume %v", key)
		bvc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	bvc.logger.WithError(err).Warnf("Dropping Longhorn backup volume %v out of the queue", key)
	bvc.queue.Forget(key)
}

func getLoggerForBackupVolume(logger logrus.FieldLogger, backupVolume *longhorn.BackupVolume) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"backupVolume": backupVolume.Name,
		},
	)
}

func (bvc *BackupVolumeController) reconcile(backupVolumeName string) (err error) {
	// Get BackupVolume CR
	backupVolume, err := bvc.ds.GetBackupVolume(backupVolumeName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Check the responsible node
	defaultEngineImage, err := bvc.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return err
	}
	isResponsible, err := bvc.isResponsibleFor(backupVolume, defaultEngineImage)
	if err != nil {
		return nil
	}
	if !isResponsible {
		return nil
	}
	if backupVolume.Status.OwnerID != bvc.controllerID {
		backupVolume.Status.OwnerID = bvc.controllerID
		backupVolume, err = bvc.ds.UpdateBackupVolumeStatus(backupVolume)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
	}

	log := getLoggerForBackupVolume(bvc.logger, backupVolume)

	// Get default backup target
	backupTarget, err := bvc.ds.GetBackupTargetRO(types.DefaultBackupTargetName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		log.Warnf("Cannot found the %s backup target", types.DefaultBackupTargetName)
		return nil
	}

	// Examine DeletionTimestamp to determine if object is under deletion
	if !backupVolume.DeletionTimestamp.IsZero() {
		if err := bvc.ds.DeleteAllBackupsForBackupVolume(backupVolumeName); err != nil {
			log.WithError(err).Error("Error deleting backups")
			return err
		}

		// Delete the backup volume from the remote backup target
		if backupTarget.Spec.BackupTargetURL != "" {
			engineClientProxy, backupTargetClient, err := getBackupTarget(bvc.controllerID, backupTarget, bvc.ds, log, bvc.proxyConnCounter)
			if err != nil || engineClientProxy == nil {
				log.WithError(err).Error("Error init backup target clients")
				return nil // Ignore error to prevent enqueue
			}
			defer engineClientProxy.Close()

			if err := backupTargetClient.BackupVolumeDelete(backupTargetClient.URL, backupVolumeName, backupTargetClient.Credential); err != nil {
				log.WithError(err).Error("Error clean up remote backup volume")
				return err
			}
		}
		return bvc.ds.RemoveFinalizerForBackupVolume(backupVolume)
	}

	syncTime := metav1.Time{Time: time.Now().UTC()}
	existingBackupVolume := backupVolume.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingBackupVolume.Status, backupVolume.Status) {
			return
		}
		if _, err := bvc.ds.UpdateBackupVolumeStatus(backupVolume); err != nil && apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", backupVolumeName)
			bvc.enqueueBackupVolume(backupVolume)
		}
	}()

	// Check the controller should run synchronization
	if !backupVolume.Status.LastSyncedAt.IsZero() &&
		!backupVolume.Spec.SyncRequestedAt.After(backupVolume.Status.LastSyncedAt.Time) {
		return nil
	}

	engineClientProxy, backupTargetClient, err := getBackupTarget(bvc.controllerID, backupTarget, bvc.ds, log, bvc.proxyConnCounter)
	if err != nil {
		log.WithError(err).Error("Error init backup target clients")
		return nil // Ignore error to prevent enqueue
	}
	defer engineClientProxy.Close()

	// Get a list of all the backups that are stored in the backup target
	res, err := backupTargetClient.BackupNameList(backupTargetClient.URL, backupVolumeName, backupTargetClient.Credential)
	if err != nil {
		log.WithError(err).Error("Error listing backups from backup target")
		return nil // Ignore error to prevent enqueue
	}
	backupStoreBackups := sets.NewString(res...)

	// Get a list of all the backups that exist as custom resources in the cluster
	clusterBackups, err := bvc.ds.ListBackupsWithBackupVolumeName(backupVolumeName)
	if err != nil {
		log.WithError(err).Error("Error listing backups in the cluster")
		return err
	}

	clustersSet := sets.NewString()
	for _, b := range clusterBackups {
		// Skip the Backup CR which is created from the local cluster and
		// the snapshot backup hasn't be completed or pulled from the remote backup target yet
		if b.Spec.SnapshotName != "" && b.Status.State != longhorn.BackupStateCompleted {
			continue
		}
		clustersSet.Insert(b.Name)
	}

	// Get a list of backups that *are* in the backup target and *aren't* in the cluster
	// and create the Backup CR in the cluster
	backupsToPull := backupStoreBackups.Difference(clustersSet)
	if count := backupsToPull.Len(); count > 0 {
		log.Infof("Found %d backups in the backup target that do not exist in the cluster and need to be pulled", count)
	}
	for backupName := range backupsToPull {
		backup := &longhorn.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name: backupName,
			},
		}
		if _, err = bvc.ds.CreateBackup(backup, backupVolumeName); err != nil && !apierrors.IsAlreadyExists(err) {
			log.WithError(err).Errorf("Error creating backup %s into cluster", backupName)
			return err
		}
	}

	// Get a list of backups that *are* in the cluster and *aren't* in the backup target
	// and delete the Backup CR in the cluster
	backupsToDelete := clustersSet.Difference(backupStoreBackups)
	if count := backupsToDelete.Len(); count > 0 {
		log.Infof("Found %d backups in the backup target that do not exist in the backup target and need to be deleted", count)
	}
	for backupName := range backupsToDelete {
		if err = bvc.ds.DeleteBackup(backupName); err != nil {
			log.WithError(err).Errorf("Error deleting backup %s from cluster", backupName)
			return err
		}
	}

	backupVolumeMetadataURL := backupstore.EncodeBackupURL("", backupVolumeName, backupTargetClient.URL)
	configMetadata, err := backupTargetClient.BackupConfigMetaGet(backupVolumeMetadataURL, backupTargetClient.Credential)
	if err != nil {
		log.WithError(err).Error("Error getting backup volume config metadata from backup target")
		return nil // Ignore error to prevent enqueue
	}
	if configMetadata == nil {
		return nil
	}

	// If there is no backup CR creation/deletion and the backup volume config metadata not changed
	// skip read the backup volume config
	if len(backupsToPull) == 0 && len(backupsToDelete) == 0 &&
		backupVolume.Status.LastModificationTime.Time.Equal(configMetadata.ModificationTime) {
		backupVolume.Status.LastSyncedAt = syncTime
		return nil
	}

	backupVolumeInfo, err := backupTargetClient.BackupVolumeGet(backupVolumeMetadataURL, backupTargetClient.Credential)
	if err != nil {
		log.WithError(err).Error("Error getting backup volume config from backup target")
		return nil // Ignore error to prevent enqueue
	}
	if backupVolumeInfo == nil {
		return nil
	}

	// Update the Backup CR spec.syncRequestAt to request the
	// backup_controller to reconcile the Backup CR if the last backup changed
	if backupVolume.Status.LastBackupName != backupVolumeInfo.LastBackupName {
		backup, err := bvc.ds.GetBackup(backupVolumeInfo.LastBackupName)
		if err == nil {
			backup.Spec.SyncRequestedAt = syncTime
			if _, err = bvc.ds.UpdateBackup(backup); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
				log.WithError(err).Errorf("Error updating backup %s spec", backup.Name)
			}
		}
	}

	// Update BackupVolume CR status
	backupVolume.Status.LastModificationTime = metav1.Time{Time: configMetadata.ModificationTime}
	backupVolume.Status.Size = backupVolumeInfo.Size
	backupVolume.Status.Labels = backupVolumeInfo.Labels
	backupVolume.Status.CreatedAt = backupVolumeInfo.Created
	backupVolume.Status.LastBackupName = backupVolumeInfo.LastBackupName
	backupVolume.Status.LastBackupAt = backupVolumeInfo.LastBackupAt
	backupVolume.Status.DataStored = backupVolumeInfo.DataStored
	backupVolume.Status.Messages = backupVolumeInfo.Messages
	backupVolume.Status.BackingImageName = backupVolumeInfo.BackingImageName
	backupVolume.Status.BackingImageChecksum = backupVolumeInfo.BackingImageChecksum
	backupVolume.Status.LastSyncedAt = syncTime
	return nil
}

func (bvc *BackupVolumeController) isResponsibleFor(bv *longhorn.BackupVolume, defaultEngineImage string) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	isResponsible := isControllerResponsibleFor(bvc.controllerID, bvc.ds, bv.Name, "", bv.Status.OwnerID)

	readyNodesWithReadyEI, err := bvc.ds.ListReadyNodesWithReadyEngineImage(defaultEngineImage)
	if err != nil {
		return false, err
	}
	// No node in the system has the default engine image in ready state
	if len(readyNodesWithReadyEI) == 0 {
		return false, nil
	}

	currentOwnerEngineAvailable, err := bvc.ds.CheckEngineImageReadiness(defaultEngineImage, bv.Status.OwnerID)
	if err != nil {
		return false, err
	}
	currentNodeEngineAvailable, err := bvc.ds.CheckEngineImageReadiness(defaultEngineImage, bvc.controllerID)
	if err != nil {
		return false, err
	}

	isPreferredOwner := currentNodeEngineAvailable && isResponsible
	continueToBeOwner := currentNodeEngineAvailable && bvc.controllerID == bv.Status.OwnerID
	requiresNewOwner := currentNodeEngineAvailable && !currentOwnerEngineAvailable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

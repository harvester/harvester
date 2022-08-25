package controller

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
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

	"github.com/longhorn/backupstore"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

var (
	// maxRetriesOnAcquireLockError should guarantee the cumulative retry time
	// is larger than 150 seconds.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a deployment is going to be requeued:
	//
	// 5ms, 10ms, 20ms, ... , 81.92s, 163.84s
	maxRetriesOnAcquireLockError = 16
)

type BackupController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	monitors    map[string]*engineapi.BackupMonitor
	monitorLock sync.RWMutex

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	proxyConnCounter util.Counter
}

func NewBackupController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
	proxyConnCounter util.Counter,
) *BackupController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	bc := &BackupController{
		baseController: newBaseController("longhorn-backup", logger),

		namespace:    namespace,
		controllerID: controllerID,

		monitors:    map[string]*engineapi.BackupMonitor{},
		monitorLock: sync.RWMutex{},

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-backup-controller"}),

		proxyConnCounter: proxyConnCounter,
	}

	ds.BackupInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    bc.enqueueBackup,
		UpdateFunc: func(old, cur interface{}) { bc.enqueueBackup(cur) },
		DeleteFunc: bc.enqueueBackup,
	})
	bc.cacheSyncs = append(bc.cacheSyncs, ds.BackupInformer.HasSynced)

	return bc
}

func (bc *BackupController) enqueueBackup(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	bc.queue.Add(key)
}

func (bc *BackupController) enqueueBackupForMonitor(key string) {
	bc.queue.Add(key)
}

func (bc *BackupController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer bc.queue.ShutDown()

	bc.logger.Infof("Start Longhorn Backup controller")
	defer bc.logger.Infof("Shutting down Longhorn Backup controller")

	if !cache.WaitForNamedCacheSync(bc.name, stopCh, bc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(bc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (bc *BackupController) worker() {
	for bc.processNextWorkItem() {
	}
}

func (bc *BackupController) processNextWorkItem() bool {
	key, quit := bc.queue.Get()
	if quit {
		return false
	}
	defer bc.queue.Done(key)
	err := bc.syncHandler(key.(string))
	bc.handleErr(err, key)
	return true
}

func (bc *BackupController) handleErr(err error, key interface{}) {
	if err == nil {
		bc.queue.Forget(key)
		return
	}

	// The resync period of the backup is one hour and the maxRetries is 3.
	// Thus, the deletion failure of the backup in error state is caused by the shutdown of the replica during backing up,
	// if the lock hold by the backup job is not released.
	// The workaround is to increase the maxRetries number and to retry the deletion until the lock acquisition
	// of the backup is timeout after 150 seconds.
	if strings.Contains(err.Error(), "failed lock") {
		if bc.queue.NumRequeues(key) < maxRetriesOnAcquireLockError {
			bc.logger.WithError(err).Warnf("Error syncing Longhorn backup %v because of the failure of lock acquisition", key)
			bc.queue.AddRateLimited(key)
			return
		}
	} else {
		if bc.queue.NumRequeues(key) < maxRetries {
			bc.logger.WithError(err).Warnf("Error syncing Longhorn backup %v", key)
			bc.queue.AddRateLimited(key)
			return
		}
	}

	utilruntime.HandleError(err)
	bc.logger.WithError(err).Warnf("Dropping Longhorn backup %v out of the queue", key)
	bc.queue.Forget(key)
}

func (bc *BackupController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync backup %v", bc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != bc.namespace {
		return nil
	}
	return bc.reconcile(name)
}

func getLoggerForBackup(logger logrus.FieldLogger, backup *longhorn.Backup) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"backup": backup.Name,
		},
	)
}

func (bc *BackupController) reconcile(backupName string) (err error) {
	// Get Backup CR
	backup, err := bc.ds.GetBackup(backupName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Check the responsible node
	defaultEngineImage, err := bc.ds.GetSettingValueExisted(types.SettingNameDefaultEngineImage)
	if err != nil {
		return err
	}
	isResponsible, err := bc.isResponsibleFor(backup, defaultEngineImage)
	if err != nil {
		return nil
	}
	if !isResponsible {
		return nil
	}
	if backup.Status.OwnerID != bc.controllerID {
		backup.Status.OwnerID = bc.controllerID
		backup, err = bc.ds.UpdateBackupStatus(backup)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
	}

	log := getLoggerForBackup(bc.logger, backup)

	// Get default backup target
	backupTarget, err := bc.ds.GetBackupTargetRO(types.DefaultBackupTargetName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		log.Warnf("Cannot found the %s backup target", types.DefaultBackupTargetName)
		return nil
	}

	// Find the backup volume name from label
	backupVolumeName, err := bc.getBackupVolumeName(backup)
	if err != nil {
		if types.ErrorIsNotFound(err) {
			return nil // Ignore error to prevent enqueue
		}
		log.WithError(err).Warning("Cannot find backup volume name")
		return err
	}

	// Examine DeletionTimestamp to determine if object is under deletion
	if !backup.DeletionTimestamp.IsZero() {
		backupVolume, err := bc.ds.GetBackupVolume(backupVolumeName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		if backupTarget.Spec.BackupTargetURL != "" &&
			backupVolume != nil && backupVolume.DeletionTimestamp == nil {
			backupTargetClient, err := getBackupTargetClient(bc.ds, backupTarget)
			if err != nil {
				log.WithError(err).Error("Error init backup target clients")
				return nil // Ignore error to prevent enqueue
			}

			backupURL := backupstore.EncodeBackupURL(backup.Name, backupVolumeName, backupTargetClient.URL)
			if err := backupTargetClient.BackupDelete(backupURL, backupTargetClient.Credential); err != nil {
				log.WithError(err).Error("Error deleting remote backup")
				return err
			}
		}

		// Request backup_volume_controller to reconcile BackupVolume immediately if it's the last backup
		if backupVolume != nil && backupVolume.Status.LastBackupName == backup.Name {
			backupVolume.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
			if _, err = bc.ds.UpdateBackupVolume(backupVolume); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
				log.WithError(err).Errorf("Error updating backup volume %s spec", backupVolumeName)
				// Do not return err to enqueue since backup_controller is responsible to
				// reconcile Backup CR spec, waits the backup_volume_controller next reconcile time
				// to update it's BackupVolume CR status
			}
		}

		// Disable monitor regardless of backup state
		bc.disableBackupMonitor(backup.Name)

		return bc.ds.RemoveFinalizerForBackup(backup)
	}

	syncTime := metav1.Time{Time: time.Now().UTC()}
	existingBackup := backup.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingBackup.Status, backup.Status) {
			return
		}
		if _, err := bc.ds.UpdateBackupStatus(backup); err != nil && apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", backupName)
			bc.enqueueBackup(backup)
		}
	}()

	// Perform backup snapshot to the remote backup target
	// If the Backup CR is created by the user/API layer (spec.snapshotName != "") and has not been synced (status.lastSyncedAt == ""),
	// it means creating a backup from a volume snapshot is required.
	// Hence the source of truth is the engine/replica and the controller needs to sync the status with it.
	// Otherwise, the Backup CR is created by the backup volume controller, which means the backup already
	// exists in the remote backup target before the CR creation.
	// What the controller needs to do for this case is retrieve the info from the remote backup target.
	if backup.Status.LastSyncedAt.IsZero() && backup.Spec.SnapshotName != "" {
		volume, err := bc.ds.GetVolume(backupVolumeName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			err = fmt.Errorf("Cannot find the corresponding volume: %v", err)
			log.WithError(err).Error()
			backup.Status.Error = err.Error()
			backup.Status.State = longhorn.BackupStateError
			backup.Status.LastSyncedAt = syncTime
			return nil // Ignore error to prevent enqueue
		}

		if backup.Status.SnapshotCreatedAt == "" || backup.Status.VolumeSize == "" {
			bc.syncBackupStatusWithSnapshotCreationTimeAndVolumeSize(volume, backup)
		}

		monitor, err := bc.checkMonitor(backup, volume, backupTarget)
		if err != nil {
			return err
		}

		if err = bc.syncWithMonitor(backup, volume, monitor); err != nil {
			return err
		}

		switch backup.Status.State {
		case longhorn.BackupStateNew, longhorn.BackupStatePending, longhorn.BackupStateInProgress:
			return nil
		case longhorn.BackupStateCompleted:
			bc.disableBackupMonitor(backup.Name)
		case longhorn.BackupStateError, longhorn.BackupStateUnknown:
			backup.Status.LastSyncedAt = syncTime
			bc.disableBackupMonitor(backup.Name)
			return nil
		}
	}

	// The backup config had synced
	if !backup.Status.LastSyncedAt.IsZero() &&
		!backup.Spec.SyncRequestedAt.After(backup.Status.LastSyncedAt.Time) {
		return nil
	}

	// The backup creation is complete, then the source of truth becomes the remote backup target
	backupTargetClient, err := getBackupTargetClient(bc.ds, backupTarget)
	if err != nil {
		log.WithError(err).Error("Error init backup target clients")
		return nil // Ignore error to prevent enqueue
	}

	backupURL := backupstore.EncodeBackupURL(backup.Name, backupVolumeName, backupTargetClient.URL)
	backupInfo, err := backupTargetClient.BackupGet(backupURL, backupTargetClient.Credential)
	if err != nil {
		if !strings.Contains(err.Error(), "in progress") {
			log.WithError(err).Error("Error inspecting backup config")
		}
		return nil // Ignore error to prevent enqueue
	}
	if backupInfo == nil {
		log.Warn("Backup info is nil")
		return nil
	}

	// Update Backup CR status
	backup.Status.State = longhorn.BackupStateCompleted
	backup.Status.URL = backupInfo.URL
	backup.Status.SnapshotName = backupInfo.SnapshotName
	backup.Status.SnapshotCreatedAt = backupInfo.SnapshotCreated
	backup.Status.BackupCreatedAt = backupInfo.Created
	backup.Status.Size = backupInfo.Size
	backup.Status.Labels = backupInfo.Labels
	backup.Status.Messages = backupInfo.Messages
	backup.Status.VolumeName = backupInfo.VolumeName
	backup.Status.VolumeSize = backupInfo.VolumeSize
	backup.Status.VolumeCreated = backupInfo.VolumeCreated
	backup.Status.VolumeBackingImageName = backupInfo.VolumeBackingImageName
	backup.Status.LastSyncedAt = syncTime
	return nil
}

func (bc *BackupController) isResponsibleFor(b *longhorn.Backup, defaultEngineImage string) (bool, error) {
	var err error
	defer func() {
		err = errors.Wrap(err, "error while checking isResponsibleFor")
	}()

	isResponsible := isControllerResponsibleFor(bc.controllerID, bc.ds, b.Name, "", b.Status.OwnerID)

	readyNodesWithReadyEI, err := bc.ds.ListReadyNodesWithReadyEngineImage(defaultEngineImage)
	if err != nil {
		return false, err
	}
	// No node in the system has the default engine image in ready state
	if len(readyNodesWithReadyEI) == 0 {
		return false, nil
	}

	currentOwnerEngineAvailable, err := bc.ds.CheckEngineImageReadiness(defaultEngineImage, b.Status.OwnerID)
	if err != nil {
		return false, err
	}
	currentNodeEngineAvailable, err := bc.ds.CheckEngineImageReadiness(defaultEngineImage, bc.controllerID)
	if err != nil {
		return false, err
	}

	isPreferredOwner := currentNodeEngineAvailable && isResponsible
	continueToBeOwner := currentNodeEngineAvailable && bc.controllerID == b.Status.OwnerID
	requiresNewOwner := currentNodeEngineAvailable && !currentOwnerEngineAvailable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

func (bc *BackupController) getBackupVolumeName(backup *longhorn.Backup) (string, error) {
	backupVolumeName, ok := backup.Labels[types.LonghornLabelBackupVolume]
	if !ok {
		return "", fmt.Errorf("cannot find the backup volume label")
	}
	return backupVolumeName, nil
}

func (bc *BackupController) getEngineBinaryClient(volumeName string) (*engineapi.EngineBinary, error) {
	engine, err := bc.ds.GetVolumeCurrentEngine(volumeName)
	if err != nil {
		return nil, err
	}
	if engine == nil {
		return nil, fmt.Errorf("cannot get the client since the engine is nil")
	}
	return GetBinaryClientForEngine(engine, &engineapi.EngineCollection{}, engine.Status.CurrentImage)
}

// validateBackingImageChecksum validates backing image checksum
func (bc *BackupController) validateBackingImageChecksum(volName, biName string) (string, error) {
	if biName == "" {
		return "", nil
	}

	bi, err := bc.ds.GetBackingImage(biName)
	if err != nil {
		return "", err
	}

	bv, err := bc.ds.GetBackupVolumeRO(volName)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}

	if bv != nil &&
		bv.Status.BackingImageChecksum != "" && bi.Status.Checksum != "" &&
		bv.Status.BackingImageChecksum != bi.Status.Checksum {
		return "", fmt.Errorf("the backing image %v checksum %v in the backup volume doesn't match the current checksum %v",
			biName, bv.Status.BackingImageChecksum, bi.Status.Checksum)
	}
	return bi.Status.Checksum, nil
}

// checkMonitor checks if the replica monitor existed.
// If yes, returns the replica monitor. Otherwise, create a new replica monitor.
func (bc *BackupController) checkMonitor(backup *longhorn.Backup, volume *longhorn.Volume, backupTarget *longhorn.BackupTarget) (*engineapi.BackupMonitor, error) {
	if backup == nil || volume == nil || backupTarget == nil {
		return nil, nil
	}

	// There is a monitor already
	if monitor := bc.hasMonitor(backup.Name); monitor != nil {
		return monitor, nil
	}

	// Backing image checksum validation
	biChecksum, err := bc.validateBackingImageChecksum(volume.Name, volume.Spec.BackingImage)
	if err != nil {
		return nil, err
	}

	engineClientProxy, backupTargetClient, err := getBackupTarget(bc.controllerID, backupTarget, bc.ds, bc.logger, bc.proxyConnCounter)
	if err != nil {
		return nil, err
	}

	// Enable the backup monitor
	monitor, err := bc.enableBackupMonitor(backup, volume, backupTargetClient, biChecksum, engineClientProxy)
	if err != nil {
		backup.Status.Error = err.Error()
		backup.Status.State = longhorn.BackupStateError
		backup.Status.LastSyncedAt = metav1.Time{Time: time.Now().UTC()}
		return nil, err
	}
	return monitor, nil
}

// syncWithMonitor syncs the backup state/progress from the replica monitor
func (bc *BackupController) syncWithMonitor(backup *longhorn.Backup, volume *longhorn.Volume, monitor *engineapi.BackupMonitor) error {
	if backup == nil || volume == nil || monitor == nil {
		return nil
	}

	existingBackupState := backup.Status.State

	backupStatus := monitor.GetBackupStatus()
	backup.Status.Progress = backupStatus.Progress
	backup.Status.URL = backupStatus.URL
	backup.Status.Error = backupStatus.Error
	backup.Status.SnapshotName = backupStatus.SnapshotName
	backup.Status.ReplicaAddress = backupStatus.ReplicaAddress
	backup.Status.State = backupStatus.State

	if existingBackupState == backup.Status.State {
		return nil
	}

	if backup.Status.Error != "" {
		bc.eventRecorder.Eventf(volume, corev1.EventTypeWarning, string(backup.Status.State),
			"Snapshot %s backup %s label %v: %s", backup.Spec.SnapshotName, backup.Name, backup.Spec.Labels, backup.Status.Error)
		return nil
	}
	bc.eventRecorder.Eventf(volume, corev1.EventTypeNormal, string(backup.Status.State),
		"Snapshot %s backup %s label %v", backup.Spec.SnapshotName, backup.Name, backup.Spec.Labels)

	if backup.Status.State == longhorn.BackupStateCompleted {
		return bc.syncBackupVolume(volume.Name)
	}
	return nil
}

// syncBackupVolume triggers the backup_volume_controller/backup_target_controller
// to run reconcile immediately
func (bc *BackupController) syncBackupVolume(volumeName string) error {
	syncTime := metav1.Time{Time: time.Now().UTC()}
	backupVolume, err := bc.ds.GetBackupVolume(volumeName)
	if err == nil {
		// Request backup_volume_controller to reconcile BackupVolume immediately.
		backupVolume.Spec.SyncRequestedAt = syncTime
		if _, err = bc.ds.UpdateBackupVolume(backupVolume); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			bc.logger.WithError(err).Errorf("Error updating backup volume %s spec", volumeName)
		}
	} else if err != nil && apierrors.IsNotFound(err) {
		// Request backup_target_controller to reconcile BackupTarget immediately.
		backupTarget, err := bc.ds.GetBackupTarget(types.DefaultBackupTargetName)
		if err != nil {
			bc.logger.WithError(err).Warn("Failed to get backup target")
			return err
		}
		backupTarget.Spec.SyncRequestedAt = syncTime
		if _, err = bc.ds.UpdateBackupTarget(backupTarget); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
			bc.logger.WithError(err).Warn("Failed to update backup target")
		}
	}
	return nil
}

func (bc *BackupController) hasMonitor(backupName string) *engineapi.BackupMonitor {
	bc.monitorLock.RLock()
	defer bc.monitorLock.RUnlock()
	return bc.monitors[backupName]
}

func (bc *BackupController) enableBackupMonitor(backup *longhorn.Backup, volume *longhorn.Volume, backupTargetClient *engineapi.BackupTargetClient, biChecksum string, engineClientProxy engineapi.EngineClientProxy) (*engineapi.BackupMonitor, error) {
	monitor := bc.hasMonitor(backup.Name)
	if monitor != nil {
		return monitor, nil
	}

	bc.monitorLock.Lock()
	defer bc.monitorLock.Unlock()

	engine, err := bc.ds.GetVolumeCurrentEngine(volume.Name)
	if err != nil {
		return nil, err
	}

	monitor, err = engineapi.NewBackupMonitor(bc.logger, backup, volume, backupTargetClient, biChecksum, engine, engineClientProxy, bc.enqueueBackupForMonitor)
	if err != nil {
		return nil, err
	}
	bc.monitors[backup.Name] = monitor
	return monitor, nil
}

func (bc *BackupController) disableBackupMonitor(backupName string) {
	monitor := bc.hasMonitor(backupName)
	if monitor == nil {
		return
	}

	bc.monitorLock.Lock()
	defer bc.monitorLock.Unlock()
	delete(bc.monitors, backupName)
	monitor.Close()
}

func (bc *BackupController) syncBackupStatusWithSnapshotCreationTimeAndVolumeSize(volume *longhorn.Volume, backup *longhorn.Backup) {
	backup.Status.VolumeSize = strconv.FormatInt(volume.Spec.Size, 10)
	engineCliClient, err := bc.getEngineBinaryClient(volume.Name)
	if err != nil {
		bc.logger.Warnf("syncBackupStatusWithSnapshotCreationTimeAndVolumeSize: failed to get engine client: %v", err)
		return
	}

	e, err := bc.ds.GetVolumeCurrentEngine(volume.Name)
	if err != nil {
		bc.logger.Warnf("syncBackupStatusWithSnapshotCreationTimeAndVolumeSize: failed to get engine: %v", err)
		return
	}

	engineClientProxy, err := engineapi.GetCompatibleClient(e, engineCliClient, bc.ds, bc.logger, bc.proxyConnCounter)
	if err != nil {
		bc.logger.Warnf("syncBackupStatusWithSnapshotCreationTimeAndVolumeSize: failed to get proxy: %v", err)
		return
	}
	defer engineClientProxy.Close()

	snap, err := engineClientProxy.SnapshotGet(e, backup.Spec.SnapshotName)
	if err != nil {
		bc.logger.Warnf("syncBackupStatusWithSnapshotCreationTimeAndVolumeSize: failed to get snapshot %v: %v", backup.Spec.SnapshotName, err)
		return
	}

	if snap == nil {
		bc.logger.Warnf("syncBackupStatusWithSnapshotCreationTimeAndVolumeSize: couldn't find the snapshot %v in volume %v ", backup.Spec.SnapshotName, volume.Name)
		return
	}

	backup.Status.SnapshotCreatedAt = snap.Created
}

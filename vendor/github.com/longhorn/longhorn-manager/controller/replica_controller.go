package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

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

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type ReplicaController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of replica
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	instanceHandler *InstanceHandler

	rebuildingLock          *sync.Mutex
	inProgressRebuildingMap map[string]struct{}
}

func NewReplicaController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace string, controllerID string) *ReplicaController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	rc := &ReplicaController{
		baseController: newBaseController("longhorn-replica", logger),

		namespace:    namespace,
		controllerID: controllerID,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-replica-controller"}),

		ds: ds,

		rebuildingLock:          &sync.Mutex{},
		inProgressRebuildingMap: map[string]struct{}{},
	}
	rc.instanceHandler = NewInstanceHandler(ds, rc, rc.eventRecorder)

	ds.ReplicaInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: rc.enqueueReplica,
		UpdateFunc: func(old, cur interface{}) {
			rc.enqueueReplica(cur)

			oldR := old.(*longhorn.Replica)
			curR := cur.(*longhorn.Replica)
			if IsRebuildingReplica(oldR) && !IsRebuildingReplica(curR) {
				rc.enqueueAllRebuildingReplicaOnCurrentNode()
			}
		},
		DeleteFunc: rc.enqueueReplica,
	})
	rc.cacheSyncs = append(rc.cacheSyncs, ds.ReplicaInformer.HasSynced)

	ds.InstanceManagerInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    rc.enqueueInstanceManagerChange,
		UpdateFunc: func(old, cur interface{}) { rc.enqueueInstanceManagerChange(cur) },
		DeleteFunc: rc.enqueueInstanceManagerChange,
	}, 0)
	rc.cacheSyncs = append(rc.cacheSyncs, ds.InstanceManagerInformer.HasSynced)

	ds.NodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    rc.enqueueNodeAddOrDelete,
		UpdateFunc: rc.enqueueNodeChange,
		DeleteFunc: rc.enqueueNodeAddOrDelete,
	}, 0)
	rc.cacheSyncs = append(rc.cacheSyncs, ds.NodeInformer.HasSynced)

	ds.BackingImageInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    rc.enqueueBackingImageChange,
		UpdateFunc: func(old, cur interface{}) { rc.enqueueBackingImageChange(cur) },
		DeleteFunc: rc.enqueueBackingImageChange,
	}, 0)
	rc.cacheSyncs = append(rc.cacheSyncs, ds.BackingImageInformer.HasSynced)

	ds.SettingInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) { rc.enqueueSettingChange(cur) },
	}, 0)
	rc.cacheSyncs = append(rc.cacheSyncs, ds.SettingInformer.HasSynced)

	return rc
}

func (rc *ReplicaController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer rc.queue.ShutDown()

	rc.logger.Info("Start Longhorn replica controller")
	defer rc.logger.Info("Shutting down Longhorn replica controller")

	if !cache.WaitForNamedCacheSync("longhorn replicas", stopCh, rc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(rc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (rc *ReplicaController) worker() {
	for rc.processNextWorkItem() {
	}
}

func (rc *ReplicaController) processNextWorkItem() bool {
	key, quit := rc.queue.Get()

	if quit {
		return false
	}
	defer rc.queue.Done(key)

	err := rc.syncReplica(key.(string))
	rc.handleErr(err, key)

	return true
}

func (rc *ReplicaController) handleErr(err error, key interface{}) {
	if err == nil {
		rc.queue.Forget(key)
		return
	}

	if rc.queue.NumRequeues(key) < maxRetries {
		rc.logger.WithError(err).Warnf("Error syncing Longhorn replica %v", key)
		rc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	rc.logger.WithError(err).Warnf("Dropping Longhorn replica %v out of the queue", key)
	rc.queue.Forget(key)
}

func getLoggerForReplica(logger logrus.FieldLogger, r *longhorn.Replica) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"replica":  r.Name,
			"nodeID":   r.Spec.NodeID,
			"dataPath": r.Spec.DataPath,
			"ownerID":  r.Status.OwnerID,
		},
	)
}

// From replica to check Node.Spec.EvictionRequested of the node
// this replica first, then check Node.Spec.Disks.EvictionRequested
func (rc *ReplicaController) isEvictionRequested(replica *longhorn.Replica) bool {
	// Return false if this replica has not been assigned to a node.
	if replica.Spec.NodeID == "" {
		return false
	}

	log := getLoggerForReplica(rc.logger, replica)

	if isDownOrDeleted, err := rc.ds.IsNodeDownOrDeleted(replica.Spec.NodeID); err != nil {
		log.WithError(err).Warn("Failed to check if node is down or deleted")
		return false
	} else if isDownOrDeleted {
		return false
	}

	node, err := rc.ds.GetNode(replica.Spec.NodeID)
	if err != nil {
		log.WithError(err).Warn("Failed to get node information")
		return false
	}

	// Check if node has been request eviction.
	if node.Spec.EvictionRequested {
		return true
	}

	// Check if disk has been request eviction.
	for diskName, diskStatus := range node.Status.DiskStatus {
		if diskStatus.DiskUUID != replica.Spec.DiskID {
			continue
		}
		diskSpec, ok := node.Spec.Disks[diskName]
		if !ok {
			log.Warnf("Cannot continue handling replica eviction since there is no spec for disk name %v on node %v", diskName, node.Name)
			return false
		}
		return diskSpec.EvictionRequested
	}

	return false
}

func (rc *ReplicaController) UpdateReplicaEvictionStatus(replica *longhorn.Replica) {
	log := getLoggerForReplica(rc.logger, replica)

	// Check if eviction has been requested on this replica
	if rc.isEvictionRequested(replica) &&
		!replica.Status.EvictionRequested {
		replica.Status.EvictionRequested = true
		log.Debug("Replica has requested eviction")
	}

	// Check if eviction has been cancelled on this replica
	if !rc.isEvictionRequested(replica) &&
		replica.Status.EvictionRequested {
		replica.Status.EvictionRequested = false
		log.Debug("Replica has cancelled eviction")
	}

}

func (rc *ReplicaController) syncReplica(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync replica for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != rc.namespace {
		// Not ours, don't do anything
		return nil
	}

	replica, err := rc.ds.GetReplica(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			log := rc.logger.WithField("replica", name)
			log.Info("Replica has been deleted")
			return nil
		}
		return err
	}
	dataPath := types.GetReplicaDataPath(replica.Spec.DiskPath, replica.Spec.DataDirectoryName)

	log := getLoggerForReplica(rc.logger, replica)

	if !rc.isResponsibleFor(replica) {
		return nil
	}
	if replica.Status.OwnerID != rc.controllerID {
		replica.Status.OwnerID = rc.controllerID
		replica, err = rc.ds.UpdateReplicaStatus(replica)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.WithField(
			"controllerID", rc.controllerID,
		).Debug("Replica controller picked up")
	}

	if replica.DeletionTimestamp != nil {
		if err := rc.DeleteInstance(replica); err != nil {
			return errors.Wrapf(err, "failed to cleanup the related replica process before deleting replica %v", replica.Name)
		}

		if replica.Spec.NodeID != "" && replica.Spec.NodeID != rc.controllerID {
			log.Warn("can't cleanup replica's data because the replica's data is not on this node")
		} else if replica.Spec.NodeID != "" {
			if replica.Spec.Active && dataPath != "" {
				// prevent accidentally deletion
				if !strings.Contains(filepath.Base(filepath.Clean(dataPath)), "-") {
					return fmt.Errorf("%v doesn't look like a replica data path", dataPath)
				}
				if err := util.RemoveHostDirectoryContent(dataPath); err != nil {
					return errors.Wrapf(err, "cannot cleanup after replica %v at %v", replica.Name, dataPath)
				}
				log.Debug("Cleanup replica completed")
			} else {
				log.Debug("Didn't cleanup replica since it's not the active one for the path or the path is empty")
			}
		}

		return rc.ds.RemoveFinalizerForReplica(replica)
	}

	existingReplica := replica.DeepCopy()
	defer func() {
		// we're going to update replica assume things changes
		if err == nil && !reflect.DeepEqual(existingReplica.Status, replica.Status) {
			_, err = rc.ds.UpdateReplicaStatus(replica)
		}
		// requeue if it's conflict
		if apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", key)
			rc.enqueueReplica(replica)
			err = nil
		}
	}()

	// Update `Replica.Status.EvictionRequested` field
	rc.UpdateReplicaEvictionStatus(replica)

	return rc.instanceHandler.ReconcileInstanceState(replica, &replica.Spec.InstanceSpec, &replica.Status.InstanceStatus)
}

func (rc *ReplicaController) enqueueReplica(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	rc.queue.Add(key)
}

func (rc *ReplicaController) CreateInstance(obj interface{}) (*longhorn.InstanceProcess, error) {
	r, ok := obj.(*longhorn.Replica)
	if !ok {
		return nil, fmt.Errorf("BUG: invalid object for replica process creation: %v", obj)
	}

	dataPath := types.GetReplicaDataPath(r.Spec.DiskPath, r.Spec.DataDirectoryName)
	if r.Spec.NodeID == "" || dataPath == "" || r.Spec.DiskID == "" || r.Spec.VolumeSize == 0 {
		return nil, fmt.Errorf("missing parameters for replica process creation: %v", r)
	}

	var err error
	backingImagePath := ""
	if r.Spec.BackingImage != "" {
		if backingImagePath, err = rc.GetBackingImagePathForReplicaStarting(r); err != nil {
			return nil, err
		}
		if backingImagePath == "" {
			return nil, nil
		}
	}

	if IsRebuildingReplica(r) {
		canStart, err := rc.CanStartRebuildingReplica(r)
		if err != nil {
			return nil, err
		}
		if !canStart {
			return nil, nil
		}
	}

	im, err := rc.ds.GetInstanceManagerByInstance(obj)
	if err != nil {
		return nil, err
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return c.ReplicaProcessCreate(r.Name, r.Spec.EngineImage, dataPath, backingImagePath, r.Spec.VolumeSize, r.Spec.RevisionCounterDisabled)
}

func (rc *ReplicaController) GetBackingImagePathForReplicaStarting(r *longhorn.Replica) (string, error) {
	log := getLoggerForReplica(rc.logger, r)

	bi, err := rc.ds.GetBackingImage(r.Spec.BackingImage)
	if err != nil {
		return "", err
	}
	if bi.Status.UUID == "" {
		log.Debugf("The requested backing image %v has not been initialized, UUID is empty", bi.Name)
		return "", nil
	}
	if bi.Spec.Disks == nil {
		log.Debugf("The requested backing image %v has not started disk file handling", bi.Name)
		return "", nil
	}
	if _, exists := bi.Spec.Disks[r.Spec.DiskID]; !exists {
		bi.Spec.Disks[r.Spec.DiskID] = ""
		log.Debugf("Replica %v will ask backing image %v to download file to node %v disk %v",
			r.Name, bi.Name, r.Spec.NodeID, r.Spec.DiskID)
		if _, err := rc.ds.UpdateBackingImage(bi); err != nil {
			return "", err
		}
		return "", nil
	}
	// bi.Spec.Disks[r.Spec.DiskID] exists
	if fileStatus, exists := bi.Status.DiskFileStatusMap[r.Spec.DiskID]; !exists || fileStatus.State != longhorn.BackingImageStateReady {
		currentBackingImageState := ""
		if fileStatus != nil {
			currentBackingImageState = string(fileStatus.State)
		}
		log.Debugf("Replica %v is waiting for backing image %v downloading file to node %v disk %v, the current state is %v",
			r.Name, bi.Name, r.Spec.NodeID, r.Spec.DiskID, currentBackingImageState)
		return "", nil
	}
	return types.GetBackingImagePathForReplicaManagerContainer(r.Spec.DiskPath, r.Spec.BackingImage, bi.Status.UUID), nil
}

func IsRebuildingReplica(r *longhorn.Replica) bool {
	return r.Spec.RebuildRetryCount != 0 && r.Spec.HealthyAt == "" && r.Spec.FailedAt == ""
}

func (rc *ReplicaController) CanStartRebuildingReplica(r *longhorn.Replica) (bool, error) {
	log := getLoggerForReplica(rc.logger, r)

	concurrentRebuildingLimit, err := rc.ds.GetSettingAsInt(types.SettingNameConcurrentReplicaRebuildPerNodeLimit)
	if err != nil {
		return false, err
	}

	// If the concurrent value is 0, Longhorn will rely on
	// skipping replica replenishment rather than blocking process launching here to disable the rebuilding.
	// Otherwise, the newly created replicas will keep hanging up there.
	if concurrentRebuildingLimit < 1 {
		return true, nil
	}

	// This is the only place in which the controller will operate
	// the in progress rebuilding replica map. Then the main reconcile loop
	// and the normal replicas will not be affected by the locking.
	rc.rebuildingLock.Lock()
	defer rc.rebuildingLock.Unlock()

	rsMap := map[string]*longhorn.Replica{}
	rs, err := rc.ds.ListReplicasByNodeRO(r.Spec.NodeID)
	if err != nil {
		return false, err
	}
	for _, replicaOnTheSameNode := range rs {
		rsMap[replicaOnTheSameNode.Name] = replicaOnTheSameNode
		// Just in case, this means the replica controller will try to recall
		// in-progress rebuilding replicas even if the longhorn manager pod is restarted.
		if IsRebuildingReplica(replicaOnTheSameNode) &&
			(replicaOnTheSameNode.Status.CurrentState == longhorn.InstanceStateStarting ||
				replicaOnTheSameNode.Status.CurrentState == longhorn.InstanceStateRunning) {
			rc.inProgressRebuildingMap[replicaOnTheSameNode.Name] = struct{}{}
		}
	}

	// Clean up the entries when the corresponding replica is no longer a
	// rebuilding one.
	for inProgressReplicaName := range rc.inProgressRebuildingMap {
		if inProgressReplicaName == r.Name {
			return true, nil
		}
		replicaOnTheSameNode, exists := rsMap[inProgressReplicaName]
		if !exists {
			delete(rc.inProgressRebuildingMap, inProgressReplicaName)
			continue
		}
		if !IsRebuildingReplica(replicaOnTheSameNode) {
			delete(rc.inProgressRebuildingMap, inProgressReplicaName)
		}
	}

	if len(rc.inProgressRebuildingMap) >= int(concurrentRebuildingLimit) {
		log.Debugf("Replica rebuildings for %+v are in progress on this node, which reaches or exceeds the concurrent limit value %v",
			rc.inProgressRebuildingMap, concurrentRebuildingLimit)
		return false, nil
	}

	rc.inProgressRebuildingMap[r.Name] = struct{}{}

	return true, nil
}

func (rc *ReplicaController) DeleteInstance(obj interface{}) error {
	r, ok := obj.(*longhorn.Replica)
	if !ok {
		return fmt.Errorf("BUG: invalid object for replica process deletion: %v", obj)
	}
	log := getLoggerForReplica(rc.logger, r)

	if err := rc.deleteInstanceWithCLIAPIVersionOne(r); err != nil {
		return err
	}

	var im *longhorn.InstanceManager
	var err error
	// Not assigned or not updated, try best to delete
	if r.Status.InstanceManagerName == "" {
		if r.Spec.NodeID == "" {
			log.Warnf("Replica %v does not set instance manager name and node ID, will skip the actual process deletion", r.Name)
			return nil
		}
		im, err = rc.ds.GetInstanceManagerByInstance(obj)
		if err != nil {
			log.Warnf("Failed to detect instance manager for replica %v, will skip the actual process deletion: %v", r.Name, err)
			return nil
		}
		log.Infof("Try best to clean up the process for replica %v in instance manager %v", r.Name, im.Name)
	} else {
		im, err = rc.ds.GetInstanceManager(r.Status.InstanceManagerName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			// The related node may be directly deleted.
			log.Warnf("The replica instance manager %v is gone during the replica instance %v deletion. Will do nothing for the deletion", r.Status.InstanceManagerName, r.Name)
			return nil
		}
	}

	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning {
		return nil
	}

	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.ProcessDelete(r.Name); err != nil && !types.ErrorIsNotFound(err) {
		return err
	}

	// Directly remove the instance from the map. Best effort.
	if im.Status.APIVersion == engineapi.IncompatibleInstanceManagerAPIVersion {
		delete(im.Status.Instances, r.Name)
		if _, err := rc.ds.UpdateInstanceManagerStatus(im); err != nil {
			return err
		}
	}

	return nil
}

func (rc *ReplicaController) deleteInstanceWithCLIAPIVersionOne(r *longhorn.Replica) (err error) {
	isCLIAPIVersionOne := false
	if r.Status.CurrentImage != "" {
		isCLIAPIVersionOne, err = rc.ds.IsEngineImageCLIAPIVersionOne(r.Status.CurrentImage)
		if err != nil {
			return err
		}
	}

	if isCLIAPIVersionOne {
		pod, err := rc.kubeClient.CoreV1().Pods(rc.namespace).Get(context.TODO(), r.Name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get pod for old replica %v", r.Name)
		}
		if apierrors.IsNotFound(err) {
			pod = nil
		}

		log := getLoggerForReplica(rc.logger, r)
		log.Debug("Prepared to delete old version replica with running pod")
		if err := rc.deleteOldReplicaPod(pod, r); err != nil {
			return err
		}
	}
	return nil
}

func (rc *ReplicaController) deleteOldReplicaPod(pod *v1.Pod, r *longhorn.Replica) (err error) {
	// pod already stopped
	if pod == nil {
		return nil
	}

	if pod.DeletionTimestamp != nil {
		if pod.DeletionGracePeriodSeconds != nil && *pod.DeletionGracePeriodSeconds != 0 {
			// force deletion in the case of node lost
			deletionDeadline := pod.DeletionTimestamp.Add(time.Duration(*pod.DeletionGracePeriodSeconds) * time.Second)
			now := time.Now().UTC()
			if now.After(deletionDeadline) {
				log := rc.logger.WithField("pod", pod.Name)
				log.Debugf("Replica pod still exists after grace period %v passed, force deletion: now %v, deadline %v",
					pod.DeletionGracePeriodSeconds, now, deletionDeadline)
				gracePeriod := int64(0)
				if err := rc.kubeClient.CoreV1().Pods(rc.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod}); err != nil {
					log.WithError(err).Debug("Failed to force deleting replica pod")
					return nil
				}
			}
		}
		return nil
	}

	if err := rc.kubeClient.CoreV1().Pods(rc.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
		rc.eventRecorder.Eventf(r, v1.EventTypeWarning, EventReasonFailedStopping, "Error stopping pod for old replica %v: %v", pod.Name, err)
		return nil
	}
	rc.eventRecorder.Eventf(r, v1.EventTypeNormal, EventReasonStop, "Stops pod for old replica %v", pod.Name)
	return nil
}

func (rc *ReplicaController) GetInstance(obj interface{}) (*longhorn.InstanceProcess, error) {
	r, ok := obj.(*longhorn.Replica)
	if !ok {
		return nil, fmt.Errorf("BUG: invalid object for replica process get: %v", obj)
	}

	var (
		im  *longhorn.InstanceManager
		err error
	)
	if r.Status.InstanceManagerName == "" {
		im, err = rc.ds.GetInstanceManagerByInstance(obj)
		if err != nil {
			return nil, err
		}
	} else {
		im, err = rc.ds.GetInstanceManager(r.Status.InstanceManagerName)
		if err != nil {
			return nil, err
		}
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return c.ProcessGet(r.Name)
}

func (rc *ReplicaController) LogInstance(ctx context.Context, obj interface{}) (*engineapi.InstanceManagerClient, *imapi.LogStream, error) {
	r, ok := obj.(*longhorn.Replica)
	if !ok {
		return nil, nil, fmt.Errorf("BUG: invalid object for replica process log: %v", obj)
	}

	im, err := rc.ds.GetInstanceManager(r.Status.InstanceManagerName)
	if err != nil {
		return nil, nil, err
	}
	c, err := engineapi.NewInstanceManagerClient(im)
	if err != nil {
		return nil, nil, err
	}

	// TODO: #2441 refactor this when we do the resource monitoring refactor
	stream, err := c.ProcessLog(ctx, r.Name)
	return c, stream, err
}

func (rc *ReplicaController) enqueueInstanceManagerChange(obj interface{}) {
	im, isInstanceManager := obj.(*longhorn.InstanceManager)
	if !isInstanceManager {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		im, ok = deletedState.Obj.(*longhorn.InstanceManager)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	imType, err := datastore.CheckInstanceManagerType(im)
	if err != nil || imType != longhorn.InstanceManagerTypeReplica {
		return
	}

	// replica's NodeID won't change, don't need to check instance manager
	replicasRO, err := rc.ds.ListReplicasByNodeRO(im.Spec.NodeID)
	if err != nil {
		getLoggerForInstanceManager(rc.logger, im).Warn("Failed to list replicas on node")
		return
	}

	for _, r := range replicasRO {
		rc.enqueueReplica(r)
	}

}

func (rc *ReplicaController) enqueueNodeAddOrDelete(obj interface{}) {
	node, ok := obj.(*longhorn.Node)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		node, ok = deletedState.Obj.(*longhorn.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	for diskName := range node.Spec.Disks {
		if diskStatus, existed := node.Status.DiskStatus[diskName]; existed {
			for replicaName := range diskStatus.ScheduledReplica {
				if replica, err := rc.ds.GetReplica(replicaName); err == nil {
					rc.enqueueReplica(replica)
				}
			}
		}
	}

}

func (rc *ReplicaController) enqueueNodeChange(oldObj, currObj interface{}) {
	oldNode, ok := oldObj.(*longhorn.Node)
	if !ok {
		deletedState, ok := oldObj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", oldObj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		oldNode, ok = deletedState.Obj.(*longhorn.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	currNode, ok := currObj.(*longhorn.Node)
	if !ok {
		deletedState, ok := currObj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", currObj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		currNode, ok = deletedState.Obj.(*longhorn.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	// if a node or disk changes its EvictionRequested, enqueue all replicas on that node/disk
	evictionRequestedChangeOnNodeLevel := currNode.Spec.EvictionRequested != oldNode.Spec.EvictionRequested
	for diskName, newDiskSpec := range currNode.Spec.Disks {
		oldDiskSpec, ok := oldNode.Spec.Disks[diskName]
		evictionRequestedChangeOnDiskLevel := !ok || (newDiskSpec.EvictionRequested != oldDiskSpec.EvictionRequested)
		if diskStatus, existed := currNode.Status.DiskStatus[diskName]; existed && (evictionRequestedChangeOnNodeLevel || evictionRequestedChangeOnDiskLevel) {
			for replicaName := range diskStatus.ScheduledReplica {
				if replica, err := rc.ds.GetReplica(replicaName); err == nil {
					rc.enqueueReplica(replica)
				}
			}
		}
	}

}

func (rc *ReplicaController) enqueueBackingImageChange(obj interface{}) {
	backingImage, ok := obj.(*longhorn.BackingImage)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}
		// use the last known state, to enqueue, dependent objects
		backingImage, ok = deletedState.Obj.(*longhorn.BackingImage)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	replicas, err := rc.ds.ListReplicasByNodeRO(rc.controllerID)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list replicas on node %v for backing image %v: %v", rc.controllerID, backingImage.Name, err))
		return
	}
	for diskUUID := range backingImage.Status.DiskFileStatusMap {
		for _, r := range replicas {
			if r.Spec.DiskID == diskUUID && r.Spec.BackingImage == backingImage.Name {
				rc.enqueueReplica(r)
			}
		}
	}

}

func (rc *ReplicaController) enqueueSettingChange(obj interface{}) {
	setting, ok := obj.(*longhorn.Setting)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		setting, ok = deletedState.Obj.(*longhorn.Setting)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	if types.SettingName(setting.Name) != types.SettingNameConcurrentReplicaRebuildPerNodeLimit {
		return
	}

	rc.enqueueAllRebuildingReplicaOnCurrentNode()

}

func (rc *ReplicaController) enqueueAllRebuildingReplicaOnCurrentNode() {
	replicas, err := rc.ds.ListReplicasByNodeRO(rc.controllerID)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list rebuilding replicas on current node %v: %v",
			rc.controllerID, err))
		return
	}
	for _, r := range replicas {
		if IsRebuildingReplica(r) {
			rc.enqueueReplica(r)
		}
	}
}

func (rc *ReplicaController) isResponsibleFor(r *longhorn.Replica) bool {
	return isControllerResponsibleFor(rc.controllerID, rc.ds, r.Name, r.Spec.NodeID, r.Status.OwnerID)
}

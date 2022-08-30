package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

type ShareManagerController struct {
	*baseController

	namespace      string
	controllerID   string
	serviceAccount string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewShareManagerController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,

	kubeClient clientset.Interface,
	namespace, controllerID, serviceAccount string) *ShareManagerController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)

	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	c := &ShareManagerController{
		baseController: newBaseController("longhorn-share-manager", logger),

		namespace:      namespace,
		controllerID:   controllerID,
		serviceAccount: serviceAccount,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-share-manager-controller"}),

		ds: ds,
	}

	// need shared volume manager informer
	ds.ShareManagerInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueShareManager,
		UpdateFunc: func(old, cur interface{}) { c.enqueueShareManager(cur) },
		DeleteFunc: c.enqueueShareManager,
	})
	c.cacheSyncs = append(c.cacheSyncs, ds.ShareManagerInformer.HasSynced)

	// need information for volumes, to be able to claim them
	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueShareManagerForVolume,
		UpdateFunc: func(old, cur interface{}) { c.enqueueShareManagerForVolume(cur) },
		DeleteFunc: c.enqueueShareManagerForVolume,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.VolumeInformer.HasSynced)

	// we are only interested in pods for which we are responsible for managing
	ds.PodInformer.AddEventHandlerWithResyncPeriod(cache.FilteringResourceEventHandler{
		FilterFunc: isShareManagerPod,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueShareManagerForPod,
			UpdateFunc: func(old, cur interface{}) { c.enqueueShareManagerForPod(cur) },
			DeleteFunc: c.enqueueShareManagerForPod,
		},
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.PodInformer.HasSynced)

	return c
}

func getLoggerForShareManager(logger logrus.FieldLogger, sm *longhorn.ShareManager) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"shareManager": sm.Name,
			"volume":       sm.Name,
			"owner":        sm.Status.OwnerID,
			"state":        sm.Status.State,
		},
	)
}

func (c *ShareManagerController) enqueueShareManager(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	c.queue.Add(key)
}

func (c *ShareManagerController) enqueueShareManagerForVolume(obj interface{}) {
	volume, isVolume := obj.(*longhorn.Volume)
	if !isVolume {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue the ShareManager
		volume, ok = deletedState.Obj.(*longhorn.Volume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non Volume object: %#v", deletedState.Obj))
			return
		}
	}

	if volume.Spec.AccessMode == longhorn.AccessModeReadWriteMany && !volume.Spec.Migratable {
		// we can queue the key directly since a share manager only manages a single volume from it's own namespace
		// and there is no need for us to retrieve the whole object, since we already know the volume name
		getLoggerForVolume(c.logger, volume).Trace("Enqueuing share manager for volume")
		key := volume.Namespace + "/" + volume.Name
		c.queue.Add(key)
		return
	}
}

func (c *ShareManagerController) enqueueShareManagerForPod(obj interface{}) {
	pod, isPod := obj.(*v1.Pod)
	if !isPod {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue the ShareManager
		pod, ok = deletedState.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non Pod object: %#v", deletedState.Obj))
			return
		}
	}

	// we can queue the key directly since a share manager only manages pods from it's own namespace
	// and there is no need for us to retrieve the whole object, since the share manager name is stored in the label
	smName := pod.Labels[types.GetLonghornLabelKey(types.LonghornLabelShareManager)]
	c.logger.WithField("pod", pod.Name).WithField("shareManager", smName).Trace("Enqueuing share manager for pod")
	key := pod.Namespace + "/" + smName
	c.queue.Add(key)

}

func isShareManagerPod(obj interface{}) bool {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return false
		}

		// use the last known state, to enqueue, dependent objects
		pod, ok = deletedState.Obj.(*v1.Pod)
		if !ok {
			return false
		}
	}

	podContainers := pod.Spec.Containers
	for _, con := range podContainers {
		if con.Name == types.LonghornLabelShareManager {
			return true
		}
	}
	return false
}

func (c *ShareManagerController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Infof("Start Longhorn share manager controller")
	defer c.logger.Infof("Shutting down Longhorn share manager controller")

	if !cache.WaitForNamedCacheSync("longhorn-share-manager-controller", stopCh, c.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (c *ShareManagerController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *ShareManagerController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)
	err := c.syncShareManager(key.(string))
	c.handleErr(err, key)
	return true
}

func (c *ShareManagerController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	if c.queue.NumRequeues(key) < maxRetries {
		c.logger.WithError(err).Warnf("Error syncing Longhorn share manager %v", key)
		c.queue.AddRateLimited(key)
		return
	}

	c.logger.WithError(err).Warnf("Dropping Longhorn share manager %v out of the queue", key)
	c.queue.Forget(key)
	utilruntime.HandleError(err)
}

func (c *ShareManagerController) syncShareManager(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "fail to sync %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != c.namespace {
		return nil
	}

	sm, err := c.ds.GetShareManager(name)
	if err != nil {
		if !datastore.ErrorIsNotFound(err) {
			c.logger.WithField("shareManager", name).WithError(err).Error("Failed to retrieve share manager from datastore")
			return err
		}

		c.logger.WithField("shareManager", name).Debug("Can't find share manager, may have been deleted")
		return nil
	}
	log := getLoggerForShareManager(c.logger, sm)

	isResponsible, err := c.isResponsibleFor(sm)
	if err != nil {
		return err
	}
	if !isResponsible {
		return nil
	}

	if sm.Status.OwnerID != c.controllerID {
		sm.Status.OwnerID = c.controllerID
		sm, err = c.ds.UpdateShareManagerStatus(sm)
		if err != nil {
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Share manager got new owner %v", c.controllerID)
		log = getLoggerForShareManager(c.logger, sm)
	}

	if sm.DeletionTimestamp != nil {
		if err := c.cleanupShareManagerPod(sm); err != nil {
			return err
		}
		return c.ds.RemoveFinalizerForShareManager(sm)
	}

	// update at the end, after the whole reconcile loop
	existingShareManager := sm.DeepCopy()
	defer func() {
		if err == nil && !reflect.DeepEqual(existingShareManager.Status, sm.Status) {
			_, err = c.ds.UpdateShareManagerStatus(sm)
		}

		if apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debug("Requeue share manager due to conflict")
			c.enqueueShareManager(sm)
			err = nil
		}
	}()

	if err = c.syncShareManagerVolume(sm); err != nil {
		return err
	}

	if err = c.syncShareManagerPod(sm); err != nil {
		return err
	}

	if err = c.syncShareManagerEndpoint(sm); err != nil {
		return err
	}

	return nil
}

func (c *ShareManagerController) syncShareManagerEndpoint(sm *longhorn.ShareManager) error {
	// running is once the pod is in ready state
	// which means the nfs server is up and running with the volume attached
	// the cluster service ip doesn't change for the lifetime of the volume
	if sm.Status.State != longhorn.ShareManagerStateRunning {
		sm.Status.Endpoint = ""
		return nil
	}

	service, err := c.ds.GetService(sm.Namespace, sm.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if service == nil {
		c.logger.Warn("missing service for share-manager, unsetting endpoint")
		sm.Status.Endpoint = ""
		return nil
	}

	sm.Status.Endpoint = fmt.Sprintf("nfs://%v/%v", service.Spec.ClusterIP, sm.Name)
	return nil
}

// isShareManagerRequiredForVolume checks if a share manager should export a volume
// a nil volume does not require a share manager
func (c *ShareManagerController) isShareManagerRequiredForVolume(volume *longhorn.Volume) bool {
	if volume == nil {
		return false
	}

	if volume.Spec.AccessMode != longhorn.AccessModeReadWriteMany {
		return false
	}

	// let the auto salvage take care of it
	if volume.Status.Robustness == longhorn.VolumeRobustnessFaulted {
		return false
	}

	// let the normal restore process take care of it
	if volume.Status.RestoreRequired {
		return false
	}

	// volume is used in maintenance mode
	if volume.Spec.DisableFrontend || volume.Status.FrontendDisabled {
		return false
	}

	// no need to expose (DR) standby volumes via a share manager
	if volume.Spec.Standby || volume.Status.IsStandby {
		return false
	}

	// no active workload, there is no need to keep the share manager around
	hasActiveWorkload := volume.Status.KubernetesStatus.LastPodRefAt == "" && volume.Status.KubernetesStatus.LastPVCRefAt == "" &&
		len(volume.Status.KubernetesStatus.WorkloadsStatus) > 0
	if !hasActiveWorkload {
		return false
	}

	return true
}

func (c ShareManagerController) detachShareManagerVolume(sm *longhorn.ShareManager) error {
	log := getLoggerForShareManager(c.logger, sm)
	volume, err := c.ds.GetVolume(sm.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		log.WithError(err).Error("failed to retrieve volume for share manager from datastore")
		return err
	} else if volume == nil {
		return nil
	}

	// we don't want to detach volumes that we don't control
	isMaintenanceMode := volume.Spec.DisableFrontend || volume.Status.FrontendDisabled
	shouldDetach := !isMaintenanceMode && volume.Spec.AccessMode == longhorn.AccessModeReadWriteMany && volume.Spec.NodeID != ""
	if shouldDetach {
		log.Infof("requesting Volume detach from node %v", volume.Spec.NodeID)
		volume.Spec.NodeID = ""
		volume, err = c.ds.UpdateVolume(volume)
		return err
	}

	return nil
}

// syncShareManagerVolume controls volume attachment and provides the following state transitions
// running -> running (do nothing)
// stopped -> stopped (rest state)
// starting, stopped, error -> starting (requires pod, volume attachment)
// starting, running, error -> stopped (no longer required, volume detachment)
// controls transitions to starting, stopped
func (c *ShareManagerController) syncShareManagerVolume(sm *longhorn.ShareManager) (err error) {
	var isNotNeeded bool
	defer func() {
		// ensure volume gets detached if share manager needs to be cleaned up and hasn't stopped yet
		// we need the isNotNeeded var so we don't accidentally detach manually attached volumes,
		// while the share manager is no longer running (only run cleanup once)
		if isNotNeeded && sm.Status.State != longhorn.ShareManagerStateStopping && sm.Status.State != longhorn.ShareManagerStateStopped {
			getLoggerForShareManager(c.logger, sm).Info("stopping share manager")
			if err = c.detachShareManagerVolume(sm); err == nil {
				sm.Status.State = longhorn.ShareManagerStateStopping
			}
		}
	}()

	log := getLoggerForShareManager(c.logger, sm)
	volume, err := c.ds.GetVolume(sm.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		log.WithError(err).Error("failed to retrieve volume for share manager from datastore")
		return err
	}

	if !c.isShareManagerRequiredForVolume(volume) {
		if sm.Status.State != longhorn.ShareManagerStateStopping && sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Info("share manager is no longer required")
			isNotNeeded = true
		}
		return nil
	} else if sm.Status.State == longhorn.ShareManagerStateRunning {
		return nil
	}

	// in a single node cluster, there is no other manager that can claim ownership so we are prevented from creation
	// of the share manager pod and need to ensure that the volume gets detached, so that the engine can be stopped as well
	// we only check for running, since we don't want to nuke valid pods, not schedulable only means no new pods.
	// in the case of a drain kubernetes will terminate the running pod, which we will mark as error in the sync pod method
	if !c.ds.IsNodeSchedulable(sm.Status.OwnerID) {
		if sm.Status.State != longhorn.ShareManagerStateStopping && sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Info("cannot start share manager, node is not schedulable")
			isNotNeeded = true
		}
		return nil
	}

	// we wait till a transition to stopped before ramp up again
	if sm.Status.State == longhorn.ShareManagerStateStopping {
		log.Debug("waiting for share manager stopped, before starting")
		return nil
	}

	// we manage volume auto detach/attach in the starting state, once the pod is running
	// the volume health check will be responsible for failing the pod which will lead to error state
	if sm.Status.State != longhorn.ShareManagerStateStarting {
		log.Debug("starting share manager")
		sm.Status.State = longhorn.ShareManagerStateStarting
	}

	// TODO: #2527 at the moment the consumer needs to control the state transitions of the volume
	//  since it's not possible to just specify the desired state, we should fix that down the line.
	//  for the actual state transition we need to request detachment then wait for detachment
	//  afterwards we can request attachment to the new node.
	isDown, err := c.ds.IsNodeDownOrDeleted(volume.Spec.NodeID)
	if volume.Spec.NodeID != "" && err != nil {
		log.WithError(err).Warnf("cannot check IsNodeDownOrDeleted(%v) when syncShareManagerVolume", volume.Spec.NodeID)
	}

	nodeSwitch := volume.Spec.NodeID != "" && volume.Spec.NodeID != sm.Status.OwnerID
	buggedVolume := volume.Spec.NodeID != "" && volume.Spec.NodeID == sm.Status.OwnerID && volume.Status.CurrentNodeID != "" && volume.Status.CurrentNodeID != volume.Spec.NodeID
	shouldDetach := isDown || buggedVolume || nodeSwitch
	if shouldDetach {
		log.Infof("Requesting Volume detach from node %v attached node %v", volume.Spec.NodeID, volume.Status.CurrentNodeID)
		if err = c.detachShareManagerVolume(sm); err != nil {
			return err
		}
	}

	// TODO: #2527 this detach/attach is so brittle, we really need to fix the volume controller
	// 	we need to wait till the volume is completely detached even though the state might be detached
	// 	it might still have a current node id set, which will then block reattachment forever
	shouldAttach := volume.Status.State == longhorn.VolumeStateDetached && volume.Spec.NodeID == "" && volume.Status.CurrentNodeID == ""
	if shouldAttach {
		log.WithField("volume", volume.Name).Info("Requesting Volume attach to share manager node")
		volume.Spec.NodeID = sm.Status.OwnerID
		if volume, err = c.ds.UpdateVolume(volume); err != nil {
			return err
		}
	}

	return nil
}

func (c *ShareManagerController) cleanupShareManagerPod(sm *longhorn.ShareManager) error {
	log := getLoggerForShareManager(c.logger, sm)
	podName := types.GetShareManagerPodNameFromShareManagerName(sm.Name)
	pod, err := c.ds.GetPod(podName)
	if err != nil && !apierrors.IsNotFound(err) {
		log.WithError(err).WithField("pod", podName).Error("failed to retrieve pod for share manager from datastore")
		return err
	}

	if pod == nil {
		return nil
	}

	if err := c.ds.DeletePod(podName); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if nodeFailed, _ := c.ds.IsNodeDownOrDeleted(pod.Spec.NodeName); nodeFailed {
		log.Debug("node of share manager pod is down, force deleting pod to allow fail over")
		gracePeriod := int64(0)
		err := c.kubeClient.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
		if err != nil && !apierrors.IsNotFound(err) {
			log.WithError(err).Debugf("failed to force delete share manager pod")
			return err
		}
	}

	return nil
}

// syncShareManagerPod controls pod existence and provides the following state transitions
// stopping -> stopped (no more pod)
// stopped -> stopped (rest state)
// starting -> starting (pending, volume attachment)
// starting ,running, error -> error (restart, remount volumes)
// starting, running -> running (share ready to use)
// controls transitions to running, error
func (c *ShareManagerController) syncShareManagerPod(sm *longhorn.ShareManager) (err error) {
	defer func() {
		if sm.Status.State == longhorn.ShareManagerStateStopping || sm.Status.State == longhorn.ShareManagerStateStopped ||
			sm.Status.State == longhorn.ShareManagerStateError {
			err = c.cleanupShareManagerPod(sm)
		}
	}()

	// if we are in stopped state there is nothing to do but cleanup any outstanding pods
	// no need for remount, since we don't have any active workloads in this state
	if sm.Status.State == longhorn.ShareManagerStateStopped {
		return nil
	}

	log := getLoggerForShareManager(c.logger, sm)
	pod, err := c.ds.GetPod(types.GetShareManagerPodNameFromShareManagerName(sm.Name))
	if err != nil && !apierrors.IsNotFound(err) {
		log.WithError(err).Error("failed to retrieve pod for share manager from datastore")
		return err
	} else if pod == nil {

		if sm.Status.State == longhorn.ShareManagerStateStopping {
			log.Debug("Share Manager pod is gone, transitioning to stopped state for share manager stopped, before starting")
			sm.Status.State = longhorn.ShareManagerStateStopped
			return nil
		}

		// there should only ever be no pod if we are in pending state
		// if there is no pod in any other state transition to error so we start over
		if sm.Status.State != longhorn.ShareManagerStateStarting {
			log.Debug("Share Manager has no pod but is not in starting state, requires cleanup with remount")
			sm.Status.State = longhorn.ShareManagerStateError
			return nil
		}

		if pod, err = c.createShareManagerPod(sm); err != nil {
			log.WithError(err).Error("failed to create pod for share manager")
			return err
		}
	}

	// If the node where the pod is running on become defective, we clean up the pod by setting sm.Status.State to STOPPED or ERROR
	// A new pod will be recreated by the share manager controller.
	isDown, err := c.ds.IsNodeDownOrDeleted(pod.Spec.NodeName)
	if err != nil {
		log.WithError(err).Warnf("cannot check IsNodeDownOrDeleted(%v) when syncShareManagerPod", pod.Spec.NodeName)
	}
	if pod.DeletionTimestamp != nil || isDown {

		// if we just transitioned to the starting state, while the prior cleanup is still in progress we will switch to error state
		// which will lead to a bad loop of starting (new workload) -> error (remount) -> stopped (cleanup sm)
		if sm.Status.State == longhorn.ShareManagerStateStopping {
			log.Debug("Share Manager is waiting for pod deletion before transitioning to stopped state")
			return nil
		}

		if sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Debug("Share Manager pod requires cleanup with remount")
			sm.Status.State = longhorn.ShareManagerStateError
		}

		return nil
	}

	// if we have an deleted pod but are supposed to be stopping
	// we don't modify the share-manager state
	if sm.Status.State == longhorn.ShareManagerStateStopping {
		log.Debug("Share Manager is waiting for pod deletion before transitioning to stopped state")
		return nil
	}

	switch pod.Status.Phase {
	case v1.PodPending:
		if sm.Status.State != longhorn.ShareManagerStateStarting {
			log.Errorf("Share Manager has state %v but the related pod is pending.", sm.Status.State)
			sm.Status.State = longhorn.ShareManagerStateError
		}
	case v1.PodRunning:
		// pod readiness is based on the availability of the nfs server
		// nfs server is only started after the volume is attached and mounted
		allContainersReady := true
		for _, st := range pod.Status.ContainerStatuses {
			allContainersReady = allContainersReady && st.Ready
		}

		if !allContainersReady {
			c.enqueueShareManager(sm)
		} else if sm.Status.State == longhorn.ShareManagerStateStarting {
			sm.Status.State = longhorn.ShareManagerStateRunning
		} else if sm.Status.State != longhorn.ShareManagerStateRunning {
			sm.Status.State = longhorn.ShareManagerStateError
		}

	default:
		sm.Status.State = longhorn.ShareManagerStateError
	}

	return nil
}

// createShareManagerPod ensures existence of service, it's assumed that the pvc for this share manager already exists
func (c *ShareManagerController) createShareManagerPod(sm *longhorn.ShareManager) (*v1.Pod, error) {
	setting, err := c.ds.GetSetting(types.SettingNameTaintToleration)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get taint toleration setting before creating share manager pod")
	}
	tolerations, err := types.UnmarshalTolerations(setting.Value)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal taint toleration setting before creating share manager pod")
	}
	nodeSelector, err := c.ds.GetSettingSystemManagedComponentsNodeSelector()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get node selector setting before creating share manager pod")
	}

	tolerationsByte, err := json.Marshal(tolerations)
	if err != nil {
		return nil, err
	}
	annotations := map[string]string{types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix): string(tolerationsByte)}

	imagePullPolicy, err := c.ds.GetSettingImagePullPolicy()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get image pull policy before creating share manager pod")
	}

	setting, err = c.ds.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get registry secret setting before creating share manager pod")
	}
	registrySecret := setting.Value

	setting, err = c.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get priority class setting before creating share manager pod")
	}
	priorityClass := setting.Value

	// check if we need to create the service
	if _, err := c.ds.GetService(c.namespace, sm.Name); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "failed to get service for share manager %v", sm.Name)
		}

		if _, err = c.ds.CreateService(c.namespace, c.createServiceManifest(sm)); err != nil {
			return nil, errors.Wrapf(err, "failed to create service for share manager %v", sm.Name)
		}
	}

	volume, err := c.ds.GetVolume(sm.Name)
	if err != nil {
		return nil, err
	}

	pv, err := c.ds.GetPersistentVolume(volume.Status.KubernetesStatus.PVName)
	if err != nil {
		return nil, err
	}

	fsType := pv.Spec.CSI.FSType
	mountOptions := pv.Spec.MountOptions

	var cryptoKey string
	if volume.Spec.Encrypted {
		secretRef := pv.Spec.CSI.NodePublishSecretRef
		secret, err := c.ds.GetSecretRO(secretRef.Namespace, secretRef.Name)
		if err != nil {
			return nil, err
		}

		cryptoKey = string(secret.Data["CRYPTO_KEY_VALUE"])
		if len(cryptoKey) == 0 {
			return nil, fmt.Errorf("missing CRYPTO_KEY_VALUE in secret for encrypted RWX volume %v", volume.Name)
		}
	}

	manifest := c.createPodManifest(sm, annotations, tolerations, imagePullPolicy, nil, registrySecret, priorityClass, nodeSelector,
		fsType, mountOptions, cryptoKey)
	pod, err := c.ds.CreatePod(manifest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pod for share manager %v", sm.Name)
	}
	getLoggerForShareManager(c.logger, sm).WithField("pod", pod.Name).Info("Created pod for share manager")
	return pod, nil
}

func (c *ShareManagerController) createServiceManifest(sm *longhorn.ShareManager) *v1.Service {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            sm.Name,
			Namespace:       c.namespace,
			OwnerReferences: datastore.GetOwnerReferencesForShareManager(sm, false),
			Labels:          types.GetShareManagerInstanceLabel(sm.Name),
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "", // we let the cluster assign a random ip
			Type:      v1.ServiceTypeClusterIP,
			Selector:  types.GetShareManagerInstanceLabel(sm.Name),
			Ports: []v1.ServicePort{
				{
					Name:     "nfs",
					Port:     2049,
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	return service
}

func (c *ShareManagerController) createPodManifest(sm *longhorn.ShareManager, annotations map[string]string, tolerations []v1.Toleration,
	pullPolicy v1.PullPolicy, resourceReq *v1.ResourceRequirements, registrySecret, priorityClass string, nodeSelector map[string]string,
	fsType string, mountOptions []string, cryptoKey string) *v1.Pod {

	// command args for the share-manager
	args := []string{"--debug", "daemon", "--volume", sm.Name}

	if len(fsType) > 0 {
		args = append(args, "--fs", fsType)
	}

	if len(mountOptions) > 0 {
		args = append(args, "--mount", strings.Join(mountOptions, ","))
	}

	privileged := true
	podSpec := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GetShareManagerPodNameFromShareManagerName(sm.Name),
			Namespace:       sm.Namespace,
			Labels:          types.GetShareManagerLabels(sm.Name, sm.Spec.Image),
			Annotations:     annotations,
			OwnerReferences: datastore.GetOwnerReferencesForShareManager(sm, true),
		},
		Spec: v1.PodSpec{
			ServiceAccountName: c.serviceAccount,
			Tolerations:        util.GetDistinctTolerations(tolerations),
			NodeSelector:       nodeSelector,
			PriorityClassName:  priorityClass,
			NodeName:           sm.Status.OwnerID,
			Containers: []v1.Container{
				{
					Name:            types.LonghornLabelShareManager,
					Image:           sm.Spec.Image,
					ImagePullPolicy: pullPolicy,
					// Command: []string{"longhorn-share-manager"},
					Args: args,
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							Exec: &v1.ExecAction{
								Command: []string{"cat", "/var/run/ganesha.pid"},
							},
						},
						InitialDelaySeconds: datastore.PodProbeInitialDelay,
						TimeoutSeconds:      datastore.PodProbeTimeoutSeconds,
						PeriodSeconds:       datastore.PodProbePeriodSeconds,
						FailureThreshold:    datastore.PodLivenessProbeFailureThreshold,
					},
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	// this is an encrypted volume the cryptoKey is base64 encoded
	if len(cryptoKey) > 0 {
		podSpec.Spec.Containers[0].Env = []v1.EnvVar{
			{
				Name:  "ENCRYPTED",
				Value: "True",
			},
			{
				Name:  "PASSPHRASE",
				Value: string(cryptoKey),
			},
		}
	}

	podSpec.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
		{
			Name:      "host-dev",
			MountPath: "/dev",
		},
		{
			Name:      "host-sys",
			MountPath: "/sys",
		},
		{
			Name:      "host-proc",
			MountPath: "/host/proc", // we use this to enter the host namespace
		},
		{
			Name:      "lib-modules",
			MountPath: "/lib/modules",
			ReadOnly:  true,
		},
	}

	podSpec.Spec.Volumes = []v1.Volume{
		{
			Name: "host-dev",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "host-sys",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
		{
			Name: "host-proc",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: "lib-modules",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
	}

	if registrySecret != "" {
		podSpec.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: registrySecret,
			},
		}
	}

	if resourceReq != nil {
		podSpec.Spec.Containers[0].Resources = *resourceReq
	}

	return podSpec
}

// isResponsibleFor in most controllers we only checks if the node of the current owner is down
// but in the case where the node is unschedulable we want to transfer ownership,
// since we will create sm pod on the sm.Status.OwnerID when the sm starts
func (c *ShareManagerController) isResponsibleFor(sm *longhorn.ShareManager) (bool, error) {
	// We prefer keeping the owner of the share manager CR to be the same node
	// where the share manager pod is running on.
	preferredOwnerID := ""
	pod, err := c.ds.GetPod(types.GetShareManagerPodNameFromShareManagerName(sm.Name))
	if err == nil && pod != nil {
		preferredOwnerID = pod.Spec.NodeName
	}

	isResponsible := isControllerResponsibleFor(c.controllerID, c.ds, sm.Name, preferredOwnerID, sm.Status.OwnerID)

	readyAndSchedulableNodes, err := c.ds.ListReadyAndSchedulableNodes()
	if err != nil {
		return false, err
	}
	if len(readyAndSchedulableNodes) == 0 {
		return isResponsible, nil
	}

	preferredOwnerSchedulable := c.ds.IsNodeSchedulable(preferredOwnerID)
	currentOwnerSchedulable := c.ds.IsNodeSchedulable(sm.Status.OwnerID)
	currentNodeSchedulable := c.ds.IsNodeSchedulable(c.controllerID)

	isPreferredOwner := currentNodeSchedulable && isResponsible
	continueToBeOwner := currentNodeSchedulable && !preferredOwnerSchedulable && c.controllerID == sm.Status.OwnerID
	requiresNewOwner := currentNodeSchedulable && !preferredOwnerSchedulable && !currentOwnerSchedulable

	return isPreferredOwner || continueToBeOwner || requiresNewOwner, nil
}

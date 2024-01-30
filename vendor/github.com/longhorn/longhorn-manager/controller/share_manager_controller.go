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

	"k8s.io/apimachinery/pkg/runtime"
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

	"github.com/longhorn/longhorn-manager/csi"
	"github.com/longhorn/longhorn-manager/csi/crypto"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
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
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-share-manager-controller"}),

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
		key := volume.Namespace + "/" + volume.Name
		c.queue.Add(key)
		return
	}
}

func (c *ShareManagerController) enqueueShareManagerForPod(obj interface{}) {
	pod, isPod := obj.(*corev1.Pod)
	if !isPod {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue the ShareManager
		pod, ok = deletedState.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained non Pod object: %#v", deletedState.Obj))
			return
		}
	}

	// we can queue the key directly since a share manager only manages pods from it's own namespace
	// and there is no need for us to retrieve the whole object, since the share manager name is stored in the label
	smName := pod.Labels[types.GetLonghornLabelKey(types.LonghornLabelShareManager)]
	key := pod.Namespace + "/" + smName
	c.queue.Add(key)

}

func isShareManagerPod(obj interface{}) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return false
		}

		// use the last known state, to enqueue, dependent objects
		pod, ok = deletedState.Obj.(*corev1.Pod)
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

	c.logger.Info("Starting Longhorn share manager controller")
	defer c.logger.Info("Shut down Longhorn share manager controller")

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

	log := c.logger.WithField("ShareManager", key)
	if c.queue.NumRequeues(key) < maxRetries {
		handleReconcileErrorLogging(log, err, "Failed to sync Longhorn share manager")
		c.queue.AddRateLimited(key)
		return
	}

	handleReconcileErrorLogging(log, err, "Dropping Longhorn share manager out of the queue")
	c.queue.Forget(key)
	utilruntime.HandleError(err)
}

func (c *ShareManagerController) syncShareManager(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync %v", key)
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
			return errors.Wrapf(err, "failed to retrieve share manager %v", name)
		}
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
	}

	if sm.DeletionTimestamp != nil {
		if err := c.cleanupShareManagerPod(sm); err != nil {
			return err
		}

		err = c.ds.DeleteConfigMap(c.namespace, types.GetConfigMapNameFromShareManagerName(sm.Name))
		if err != nil && !datastore.ErrorIsNotFound(err) {
			return errors.Wrapf(err, "failed to delete the configmap (recovery backend) for share manager %v", sm.Name)
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

	log := getLoggerForShareManager(c.logger, sm)
	if service == nil {
		log.Warn("Unsetting endpoint due to missing service for share-manager")
		sm.Status.Endpoint = ""
		return nil
	}
	endpoint := service.Spec.ClusterIP
	if service.Spec.IPFamilies[0] == corev1.IPv6Protocol {
		endpoint = fmt.Sprintf("[%v]", endpoint)
	}

	sm.Status.Endpoint = fmt.Sprintf("nfs://%v/%v", endpoint, sm.Name)
	return nil
}

// isShareManagerRequiredForVolume checks if a share manager should export a volume
// a nil volume does not require a share manager
func (c *ShareManagerController) isShareManagerRequiredForVolume(sm *longhorn.ShareManager, volume *longhorn.Volume, va *longhorn.VolumeAttachment) bool {
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

	for _, attachmentTicket := range va.Spec.AttachmentTickets {
		if isRegularRWXVolume(volume) && isCSIAttacherTicket(attachmentTicket) {
			return true
		}
	}
	return false
}

func (c *ShareManagerController) createShareManagerAttachmentTicket(sm *longhorn.ShareManager, va *longhorn.VolumeAttachment) {
	log := getLoggerForShareManager(c.logger, sm)
	shareManagerAttachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeShareManagerController, sm.Name)
	shareManagerAttachmentTicket, ok := va.Spec.AttachmentTickets[shareManagerAttachmentTicketID]
	if !ok {
		//create new one
		shareManagerAttachmentTicket = &longhorn.AttachmentTicket{
			ID:     shareManagerAttachmentTicketID,
			Type:   longhorn.AttacherTypeShareManagerController,
			NodeID: sm.Status.OwnerID,
			Parameters: map[string]string{
				longhorn.AttachmentParameterDisableFrontend: longhorn.FalseValue,
			},
		}
	}
	if shareManagerAttachmentTicket.NodeID != sm.Status.OwnerID {
		log.Infof("Attachment ticket %v request a new node %v from old node %v", shareManagerAttachmentTicket.ID, sm.Status.OwnerID, shareManagerAttachmentTicket.NodeID)
		shareManagerAttachmentTicket.NodeID = sm.Status.OwnerID
	}

	va.Spec.AttachmentTickets[shareManagerAttachmentTicketID] = shareManagerAttachmentTicket
}

// unmountShareManagerVolume unmounts the volume in the share manager pod.
// It is a best effort operation and will not return an error if it fails.
func (c *ShareManagerController) unmountShareManagerVolume(sm *longhorn.ShareManager) {
	log := getLoggerForShareManager(c.logger, sm)

	podName := types.GetShareManagerPodNameFromShareManagerName(sm.Name)
	pod, err := c.ds.GetPod(podName)
	if err != nil && !apierrors.IsNotFound(err) {
		log.WithError(err).Errorf("Failed to retrieve pod %v for share manager from datastore", podName)
		return
	}

	if pod == nil {
		return
	}

	log.Infof("Unmounting volume in share manager pod")

	client, err := engineapi.NewShareManagerClient(sm, pod)
	if err != nil {
		log.WithError(err).Errorf("Failed to create share manager client for pod %v", podName)
		return
	}
	defer client.Close()

	if err := client.Unmount(); err != nil {
		log.WithError(err).Warnf("Failed to unmount share manager pod %v", podName)
	}
}

// mountShareManagerVolume checks, exports and mounts the volume in the share manager pod.
func (c *ShareManagerController) mountShareManagerVolume(sm *longhorn.ShareManager) error {
	podName := types.GetShareManagerPodNameFromShareManagerName(sm.Name)
	pod, err := c.ds.GetPod(podName)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to retrieve pod %v for share manager from datastore", podName)
	}

	if pod == nil {
		return fmt.Errorf("pod %v for share manager not found", podName)
	}

	client, err := engineapi.NewShareManagerClient(sm, pod)
	if err != nil {
		return errors.Wrapf(err, "failed to create share manager client for pod %v", podName)
	}
	defer client.Close()

	if err := client.Mount(); err != nil {
		return errors.Wrapf(err, "failed to mount share manager pod %v", podName)
	}

	return nil
}

func (c *ShareManagerController) detachShareManagerVolume(sm *longhorn.ShareManager, va *longhorn.VolumeAttachment) {
	log := getLoggerForShareManager(c.logger, sm)

	shareManagerAttachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeShareManagerController, sm.Name)
	if _, ok := va.Spec.AttachmentTickets[shareManagerAttachmentTicketID]; ok {
		log.Infof("Removing volume attachment ticket: %v to detach the volume %v", shareManagerAttachmentTicketID, va.Name)
		delete(va.Spec.AttachmentTickets, shareManagerAttachmentTicketID)
	}
}

// syncShareManagerVolume controls volume attachment and provides the following state transitions
// running -> running (do nothing)
// stopped -> stopped (rest state)
// starting, stopped, error -> starting (requires pod, volume attachment)
// starting, running, error -> stopped (no longer required, volume detachment)
// controls transitions to starting, stopped
func (c *ShareManagerController) syncShareManagerVolume(sm *longhorn.ShareManager) (err error) {
	log := getLoggerForShareManager(c.logger, sm)
	volume, err := c.ds.GetVolume(sm.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if sm.Status.State != longhorn.ShareManagerStateStopped {
				log.Infof("Stopping share manager because the volume %v is not found", sm.Name)
				sm.Status.State = longhorn.ShareManagerStateStopping
			}
			return nil
		}
		return errors.Wrap(err, "failed to retrieve volume for share manager from datastore")
	}

	va, err := c.ds.GetLHVolumeAttachmentByVolumeName(volume.Name)
	if err != nil {
		return err
	}
	existingVA := va.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingVA.Spec, va.Spec) {
			return
		}

		if _, err = c.ds.UpdateLHVolumeAttachment(va); err != nil {
			return
		}
	}()

	if !c.isShareManagerRequiredForVolume(sm, volume, va) {
		c.unmountShareManagerVolume(sm)

		c.detachShareManagerVolume(sm, va)
		if sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Info("Stopping share manager since it is no longer required")
			sm.Status.State = longhorn.ShareManagerStateStopping
		}
		return nil
	} else if sm.Status.State == longhorn.ShareManagerStateRunning {
		return c.mountShareManagerVolume(sm)
	}

	// in a single node cluster, there is no other manager that can claim ownership so we are prevented from creation
	// of the share manager pod and need to ensure that the volume gets detached, so that the engine can be stopped as well
	// we only check for running, since we don't want to nuke valid pods, not schedulable only means no new pods.
	// in the case of a drain kubernetes will terminate the running pod, which we will mark as error in the sync pod method
	if !c.ds.IsNodeSchedulable(sm.Status.OwnerID) {
		c.unmountShareManagerVolume(sm)

		c.detachShareManagerVolume(sm, va)
		if sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Info("Failed to start share manager, node is not schedulable")
			sm.Status.State = longhorn.ShareManagerStateStopping
		}
		return nil
	}

	// we wait till a transition to stopped before ramp up again
	if sm.Status.State == longhorn.ShareManagerStateStopping {
		return nil
	}

	// we manage volume auto detach/attach in the starting state, once the pod is running
	// the volume health check will be responsible for failing the pod which will lead to error state
	if sm.Status.State != longhorn.ShareManagerStateStarting {
		log.Info("Starting share manager")
		sm.Status.State = longhorn.ShareManagerStateStarting
	}

	// For the RWX volume attachment, VolumeAttachment controller will not directly handle
	// the tickets from the CSI plugin. Instead, ShareManager controller will add a
	// AttacherTypeShareManagerController ticket (as the summarization of CSI tickets) then
	// the VolumeAttachment controller is responsible for handling the AttacherTypeShareManagerController
	// tickets only. See more at https://github.com/longhorn/longhorn-manager/pull/1541#issuecomment-1429044946
	c.createShareManagerAttachmentTicket(sm, va)

	return nil
}

func (c *ShareManagerController) cleanupShareManagerPod(sm *longhorn.ShareManager) error {
	log := getLoggerForShareManager(c.logger, sm)
	podName := types.GetShareManagerPodNameFromShareManagerName(sm.Name)
	pod, err := c.ds.GetPod(podName)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to retrieve pod %v for share manager from datastore", podName)
	}

	if pod == nil {
		return nil
	}

	log.Infof("Deleting share manager pod")
	if err := c.ds.DeletePod(podName); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if nodeFailed, _ := c.ds.IsNodeDownOrDeleted(pod.Spec.NodeName); nodeFailed {
		log.Info("Force deleting pod to allow fail over since node of share manager pod is down")
		gracePeriod := int64(0)
		err := c.kubeClient.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to force delete share manager pod")
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
		if sm.Status.State == longhorn.ShareManagerStateStopping ||
			sm.Status.State == longhorn.ShareManagerStateStopped ||
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
		return errors.Wrap(err, "failed to retrieve pod for share manager from datastore")
	} else if pod == nil {
		if sm.Status.State == longhorn.ShareManagerStateStopping {
			log.Info("Updating share manager to stopped state before starting since share manager pod is gone")
			sm.Status.State = longhorn.ShareManagerStateStopped
			return nil
		}

		// there should only ever be no pod if we are in pending state
		// if there is no pod in any other state transition to error so we start over
		if sm.Status.State != longhorn.ShareManagerStateStarting {
			log.Info("Updating share manager to error state since it has no pod but is not in starting state, requires cleanup with remount")
			sm.Status.State = longhorn.ShareManagerStateError
			return nil
		}

		if pod, err = c.createShareManagerPod(sm); err != nil {
			return errors.Wrap(err, "failed to create pod for share manager")
		}
	}

	// If the node where the pod is running on become defective, we clean up the pod by setting sm.Status.State to STOPPED or ERROR
	// A new pod will be recreated by the share manager controller.
	isDown, err := c.ds.IsNodeDownOrDeleted(pod.Spec.NodeName)
	if err != nil {
		log.WithError(err).Warnf("Failed to check IsNodeDownOrDeleted(%v) when syncShareManagerPod", pod.Spec.NodeName)
	} else if isDown {
		log.Infof("Node %v is down", pod.Spec.NodeName)
	}
	if pod.DeletionTimestamp != nil || isDown {
		// if we just transitioned to the starting state, while the prior cleanup is still in progress we will switch to error state
		// which will lead to a bad loop of starting (new workload) -> error (remount) -> stopped (cleanup sm)
		if sm.Status.State == longhorn.ShareManagerStateStopping {
			return nil
		}

		if sm.Status.State != longhorn.ShareManagerStateStopped {
			log.Info("Updating share manager to error state, requires cleanup with remount")
			sm.Status.State = longhorn.ShareManagerStateError
		}

		return nil
	}

	// if we have an deleted pod but are supposed to be stopping
	// we don't modify the share-manager state
	if sm.Status.State == longhorn.ShareManagerStateStopping {
		return nil
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		if sm.Status.State != longhorn.ShareManagerStateStarting {
			log.Warnf("Share Manager has state %v but the related pod is pending.", sm.Status.State)
			sm.Status.State = longhorn.ShareManagerStateError
		}
	case corev1.PodRunning:
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
func (c *ShareManagerController) createShareManagerPod(sm *longhorn.ShareManager) (*corev1.Pod, error) {
	setting, err := c.ds.GetSetting(types.SettingNameTaintToleration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get taint toleration setting before creating share manager pod")
	}
	tolerations, err := types.UnmarshalTolerations(setting.Value)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal taint toleration setting before creating share manager pod")
	}
	nodeSelector, err := c.ds.GetSettingSystemManagedComponentsNodeSelector()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node selector setting before creating share manager pod")
	}

	tolerationsByte, err := json.Marshal(tolerations)
	if err != nil {
		return nil, err
	}
	annotations := map[string]string{types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix): string(tolerationsByte)}

	imagePullPolicy, err := c.ds.GetSettingImagePullPolicy()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get image pull policy before creating share manager pod")
	}

	setting, err = c.ds.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get registry secret setting before creating share manager pod")
	}
	registrySecret := setting.Value

	setting, err = c.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get priority class setting before creating share manager pod")
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
	var cryptoParams *crypto.EncryptParams
	if volume.Spec.Encrypted {
		secretRef := pv.Spec.CSI.NodePublishSecretRef
		secret, err := c.ds.GetSecretRO(secretRef.Namespace, secretRef.Name)
		if err != nil {
			return nil, err
		}

		cryptoKey = string(secret.Data[csi.CryptoKeyValue])
		if len(cryptoKey) == 0 {
			return nil, fmt.Errorf("missing %v in secret for encrypted RWX volume %v", csi.CryptoKeyValue, volume.Name)
		}
		cryptoParams = crypto.NewEncryptParams(
			string(secret.Data[csi.CryptoKeyProvider]),
			string(secret.Data[csi.CryptoKeyCipher]),
			string(secret.Data[csi.CryptoKeyHash]),
			string(secret.Data[csi.CryptoKeySize]),
			string(secret.Data[csi.CryptoPBKDF]))
	}

	manifest := c.createPodManifest(sm, annotations, tolerations, imagePullPolicy, nil, registrySecret, priorityClass, nodeSelector,
		fsType, mountOptions, cryptoKey, cryptoParams)
	pod, err := c.ds.CreatePod(manifest)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pod for share manager %v", sm.Name)
	}
	getLoggerForShareManager(c.logger, sm).WithField("pod", pod.Name).Info("Created pod for share manager")
	return pod, nil
}

func (c *ShareManagerController) createServiceManifest(sm *longhorn.ShareManager) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            sm.Name,
			Namespace:       c.namespace,
			OwnerReferences: datastore.GetOwnerReferencesForShareManager(sm, false),
			Labels:          types.GetShareManagerInstanceLabel(sm.Name),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "", // we let the cluster assign a random ip
			Type:      corev1.ServiceTypeClusterIP,
			Selector:  types.GetShareManagerInstanceLabel(sm.Name),
			Ports: []corev1.ServicePort{
				{
					Name:     "nfs",
					Port:     2049,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	return service
}

func (c *ShareManagerController) createPodManifest(sm *longhorn.ShareManager, annotations map[string]string, tolerations []corev1.Toleration,
	pullPolicy corev1.PullPolicy, resourceReq *corev1.ResourceRequirements, registrySecret, priorityClass string, nodeSelector map[string]string,
	fsType string, mountOptions []string, cryptoKey string, cryptoParams *crypto.EncryptParams) *corev1.Pod {

	// command args for the share-manager
	args := []string{"--debug", "daemon", "--volume", sm.Name}

	if len(fsType) > 0 {
		args = append(args, "--fs", fsType)
	}

	if len(mountOptions) > 0 {
		args = append(args, "--mount", strings.Join(mountOptions, ","))
	}

	privileged := true
	podSpec := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GetShareManagerPodNameFromShareManagerName(sm.Name),
			Namespace:       sm.Namespace,
			Labels:          types.GetShareManagerLabels(sm.Name, sm.Spec.Image),
			Annotations:     annotations,
			OwnerReferences: datastore.GetOwnerReferencesForShareManager(sm, true),
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: c.serviceAccount,
			Tolerations:        util.GetDistinctTolerations(tolerations),
			NodeSelector:       nodeSelector,
			PriorityClassName:  priorityClass,
			Containers: []corev1.Container{
				{
					Name:            types.LonghornLabelShareManager,
					Image:           sm.Spec.Image,
					ImagePullPolicy: pullPolicy,
					// Command: []string{"longhorn-share-manager"},
					Args: args,
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"cat", "/var/run/ganesha.pid"},
							},
						},
						InitialDelaySeconds: datastore.PodProbeInitialDelay,
						TimeoutSeconds:      datastore.PodProbeTimeoutSeconds,
						PeriodSeconds:       datastore.PodProbePeriodSeconds,
						FailureThreshold:    datastore.PodLivenessProbeFailureThreshold,
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// this is an encrypted volume the cryptoKey is base64 encoded
	if len(cryptoKey) > 0 {
		podSpec.Spec.Containers[0].Env = []corev1.EnvVar{
			{
				Name:  "ENCRYPTED",
				Value: "True",
			},
			{
				Name:  "PASSPHRASE",
				Value: string(cryptoKey),
			},
			{
				Name:  "CRYPTOKEYCIPHER",
				Value: string(cryptoParams.GetKeyCipher()),
			},
			{
				Name:  "CRYPTOKEYHASH",
				Value: string(cryptoParams.GetKeyHash()),
			},
			{
				Name:  "CRYPTOKEYSIZE",
				Value: string(cryptoParams.GetKeySize()),
			},
			{
				Name:  "CRYPTOPBKDF",
				Value: string(cryptoParams.GetPBKDF()),
			},
		}
	}

	podSpec.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
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

	podSpec.Spec.Volumes = []corev1.Volume{
		{
			Name: "host-dev",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "host-sys",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
		{
			Name: "host-proc",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: "lib-modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
	}

	if registrySecret != "" {
		podSpec.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
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

package controller

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/kubernetes/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	bimtypes "github.com/longhorn/backing-image-manager/pkg/types"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	BackingImageDataSourcePodContainerName = "backing-image-data-source"
)

type BackingImageDataSourceController struct {
	*baseController

	namespace      string
	controllerID   string
	serviceAccount string
	bimImageName   string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	backoff *flowcontrol.Backoff

	cacheSyncs []cache.InformerSynced

	lock       *sync.RWMutex
	monitorMap map[string]chan struct{}

	proxyConnCounter util.Counter
}

type BackingImageDataSourceMonitor struct {
	Name       string
	retryCount int
	client     *engineapi.BackingImageDataSourceClient

	stopCh       chan struct{}
	controllerID string
	log          *logrus.Entry
	ds           *datastore.DataStore
	backoff      *flowcontrol.Backoff
}

func NewBackingImageDataSourceController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace, controllerID, serviceAccount, imageManagerImage string,
	proxyConnCounter util.Counter,
) *BackingImageDataSourceController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events("")})

	c := &BackingImageDataSourceController{
		baseController: newBaseController("longhorn-backing-image-data-source", logger),

		namespace:      namespace,
		controllerID:   controllerID,
		serviceAccount: serviceAccount,
		bimImageName:   imageManagerImage,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-backing-image-data-source-controller"}),

		ds: ds,

		backoff: flowcontrol.NewBackOff(time.Minute, time.Minute*5),

		lock:       &sync.RWMutex{},
		monitorMap: map[string]chan struct{}{},

		proxyConnCounter: proxyConnCounter,
	}

	ds.BackingImageDataSourceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueBackingImageDataSource,
		UpdateFunc: func(old, cur interface{}) { c.enqueueBackingImageDataSource(cur) },
		DeleteFunc: c.enqueueBackingImageDataSource,
	})
	c.cacheSyncs = append(c.cacheSyncs, ds.BackingImageDataSourceInformer.HasSynced)

	ds.BackingImageInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueForBackingImage,
		UpdateFunc: func(old, cur interface{}) { c.enqueueForBackingImage(cur) },
		DeleteFunc: c.enqueueForBackingImage,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.BackingImageInformer.HasSynced)

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) { c.enqueueForVolume(cur) },
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.VolumeInformer.HasSynced)

	ds.NodeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, cur interface{}) { c.enqueueForLonghornNode(cur) },
		DeleteFunc: c.enqueueForLonghornNode,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.NodeInformer.HasSynced)

	ds.PodInformer.AddEventHandlerWithResyncPeriod(cache.FilteringResourceEventHandler{
		FilterFunc: isBackingImageDataSourcePod,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueForBackingImageDataSourcePod,
			UpdateFunc: func(old, cur interface{}) { c.enqueueForBackingImageDataSourcePod(cur) },
			DeleteFunc: c.enqueueForBackingImageDataSourcePod,
		},
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.PodInformer.HasSynced)

	return c
}

func (c *BackingImageDataSourceController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	logrus.Info("Starting Longhorn backing image data source controller")
	defer logrus.Info("Shut down Longhorn backing image data source controller")

	if !cache.WaitForNamedCacheSync("longhorn backing image data source", stopCh, c.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (c *BackingImageDataSourceController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *BackingImageDataSourceController) processNextWorkItem() bool {
	key, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncBackingImageDataSource(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *BackingImageDataSourceController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	log := c.logger.WithField("BackingImageDataSource", key)
	if c.queue.NumRequeues(key) < maxRetries {
		handleReconcileErrorLogging(log, err, "Failed to sync Longhorn backing image data source")
		c.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	handleReconcileErrorLogging(log, err, "Dropping Longhorn backing image data source out of the queue")
	c.queue.Forget(key)
}

func getLoggerForBackingImageDataSource(logger logrus.FieldLogger, bids *longhorn.BackingImageDataSource) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"backingImageDataSource": bids.Name,
			"nodeID":                 bids.Spec.NodeID,
			"diskUUID":               bids.Spec.DiskUUID,
			"sourceType":             bids.Spec.SourceType,
			"parameters":             bids.Spec.Parameters,
		},
	)
}

func (c *BackingImageDataSourceController) getEngineClientProxy(e *longhorn.Engine) (engineapi.EngineClientProxy, error) {
	engineCollection := &engineapi.EngineCollection{}
	engineCliClient, err := engineCollection.NewEngineClient(&engineapi.EngineClientRequest{
		EngineImage: e.Status.CurrentImage,
		VolumeName:  e.Spec.VolumeName,
		IP:          e.Status.IP,
		Port:        e.Status.Port,
	})
	if err != nil {
		return nil, err
	}

	return engineapi.GetCompatibleClient(e, engineCliClient, c.ds, c.logger, c.proxyConnCounter)
}

func (c *BackingImageDataSourceController) syncBackingImageDataSource(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to sync backing image data source for %v", key)
	}()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != c.namespace {
		return nil
	}

	bids, err := c.ds.GetBackingImageDataSource(name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to get backing image data source")
	}

	log := getLoggerForBackingImageDataSource(c.logger, bids)

	if !c.isResponsibleFor(bids) {
		return nil
	}
	if bids.Status.OwnerID != c.controllerID {
		bids.Status.OwnerID = c.controllerID
		bids, err = c.ds.UpdateBackingImageDataSourceStatus(bids)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Backing image data source got new owner %v", c.controllerID)
	}

	if bids.DeletionTimestamp != nil {
		if err := c.cleanup(bids); err != nil {
			return err
		}

		// if it is not transferred, we need to wait until it is failed-and-cleanup
		if !bids.Spec.FileTransferred {
			if bids.Status.CurrentState == longhorn.BackingImageStateFailedAndCleanUp {
				return c.ds.RemoveFinalizerForBackingImageDataSource(bids)
			}

			// if bids is not transferred
			// mark the status to failed so manager can clean up the tmp file and mark it as failed-and-cleanup
			bids.Status.Message = "backing image is deleted, requesting manager to clean up the tmp file of backing image data source"
			bids.Status.CurrentState = longhorn.BackingImageStateFailed
			if _, err = c.ds.UpdateBackingImageDataSourceStatus(bids); err != nil {
				return err
			}

			return nil
		}

		// if it is transferred, we don't need to clean up
		return c.ds.RemoveFinalizerForBackingImageDataSource(bids)
	}

	existingBIDS := bids.DeepCopy()
	defer func() {
		if err != nil && strings.Contains(err.Error(), "need to wait for volume") {
			log.WithError(err).Warnf("Need to wait for volume attachment before handling key %v", key)
			// Should ignore this error and continue update
			err = nil
		}
		if err == nil && !reflect.DeepEqual(existingBIDS.Status, bids.Status) {
			_, err = c.ds.UpdateBackingImageDataSourceStatus(bids)
		}
		if apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf("Requeue %v due to conflict", key)
			c.enqueueBackingImageDataSource(bids)
			err = nil
		}
	}()

	if err := c.syncBackingImage(bids); err != nil {
		return err
	}

	if bids.Spec.FileTransferred {
		bids.Status.CurrentState = longhorn.BackingImageStateReady
		return c.cleanup(bids)
	}

	node, diskName, err := c.ds.GetReadyDiskNode(bids.Spec.DiskUUID)
	if err != nil && !types.ErrorIsNotFound(err) {
		return err
	}
	noReadyDisk := node == nil
	diskMigrated := node != nil && (node.Name != bids.Spec.NodeID || node.Spec.Disks[diskName].Path != bids.Spec.DiskPath)
	if noReadyDisk || diskMigrated {
		if bids.Status.CurrentState != longhorn.BackingImageStateUnknown {
			bids.Status.CurrentState = longhorn.BackingImageStateUnknown
		}
		return nil
	}

	if err := c.syncBackingImageDataSourcePod(bids); err != nil {
		return err
	}

	return nil
}

func (c *BackingImageDataSourceController) cleanup(bids *longhorn.BackingImageDataSource) (err error) {
	log := getLoggerForBackingImageDataSource(c.logger, bids)

	if c.isMonitoring(bids.Name) {
		c.stopMonitoring(bids.Name)
	}

	if err := c.handleAttachmentTicketDeletion(bids); err != nil {
		return err
	}

	pod, err := c.ds.GetPod(types.GetBackingImageDataSourcePodName(bids.Name))
	if err != nil {
		return errors.Wrapf(err, "failed to get pod for backing image data source %v", bids.Name)
	}
	if pod != nil && pod.DeletionTimestamp == nil {
		log.Info("Cleaning up pod for backing image data source")
		if err := c.ds.DeletePod(pod.Name); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	bids.Status.StorageIP = ""
	bids.Status.IP = ""

	return nil
}

func (c *BackingImageDataSourceController) syncBackingImage(bids *longhorn.BackingImageDataSource) (err error) {
	// TODO: HA backing image
	bi, err := c.ds.GetBackingImage(bids.Name)
	if err != nil {
		return err
	}

	existingBI := bi.DeepCopy()
	defer func() {
		if !reflect.DeepEqual(existingBI.Spec, bi.Spec) {
			if bi, err = c.ds.UpdateBackingImage(bi); err != nil {
				return
			}
		}
	}()

	if bi.Spec.Disks == nil {
		bi.Spec.Disks = map[string]string{}
	}

	if !bids.Spec.FileTransferred {
		if _, exists := bi.Spec.Disks[bids.Spec.DiskUUID]; !exists {
			bi.Spec.Disks[bids.Spec.DiskUUID] = ""
		}
	}

	return nil
}

func (c *BackingImageDataSourceController) syncBackingImageDataSourcePod(bids *longhorn.BackingImageDataSource) (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to sync backing image data source pod")
	}()
	log := getLoggerForBackingImageDataSource(c.logger, bids)

	newBackingImageDataSource := bids.Status.CurrentState == ""

	podName := types.GetBackingImageDataSourcePodName(bids.Name)
	pod, err := c.ds.GetPod(podName)
	if err != nil {
		return errors.Wrapf(err, "failed to get pod for backing image data source %v", bids.Name)
	}
	podReady := false
	podFailed := false
	podNotReadyMessage := ""
	if pod == nil {
		podNotReadyMessage = "cannot find the pod dedicated to prepare the first backing image file"
	} else if pod.Spec.NodeName != bids.Spec.NodeID {
		podNotReadyMessage = fmt.Sprintf("pod spec node ID %v doesn't match the desired node ID %v", pod.Spec.NodeName, bids.Spec.NodeID)
	} else if pod.DeletionTimestamp != nil {
		podNotReadyMessage = "the pod dedicated to prepare the first backing image file is being deleted"
	} else if pod.Spec.Containers[0].Image != c.bimImageName {
		podNotReadyMessage = "the pod image is not the default one"
	} else {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			podReady = true
			for _, st := range pod.Status.ContainerStatuses {
				if !st.Ready {
					podReady = false
					podNotReadyMessage = fmt.Sprintf("pod phase %v but the containers not ready", corev1.PodRunning)
					break
				}
			}
		case corev1.PodFailed:
			podFailed = true
		default:
			podNotReadyMessage = fmt.Sprintf("pod phase %v", pod.Status.Phase)
		}
	}

	if podReady {
		storageIP := c.ds.GetStorageIPFromPod(pod)
		if bids.Status.StorageIP != storageIP {
			bids.Status.StorageIP = storageIP
		}
		if bids.Status.IP != pod.Status.PodIP {
			bids.Status.IP = pod.Status.PodIP
		}
		if !c.isMonitoring(bids.Name) {
			c.startMonitoring(bids)
		}
	} else {
		bids.Status.StorageIP = ""
		bids.Status.IP = ""
		if bids.Status.CurrentState != longhorn.BackingImageStateFailed &&
			bids.Status.CurrentState != longhorn.BackingImageStateFailedAndCleanUp {
			if podFailed {
				podLog := ""
				podLogBytes, err := c.ds.GetPodContainerLog(podName, BackingImageDataSourcePodContainerName)
				if err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					}
					podLog = "pod log is not found"
				} else {
					podLog = string(podLogBytes)
				}
				log.Errorf("Backing image data source was state %v but the pod failed, the state will be updated to %v, message: %s", bids.Status.CurrentState, longhorn.BackingImageStateFailed, podLog)
				bids.Status.Message = fmt.Sprintf("the pod dedicated to prepare the first backing image file failed: %s", podLog)
				bids.Status.CurrentState = longhorn.BackingImageStateFailed
			} else {
				// File processing started implicitly means the pod should have become ready.
				fileProcessingStarted :=
					bids.Status.CurrentState == longhorn.BackingImageStateStarting ||
						bids.Status.CurrentState == longhorn.BackingImageStateInProgress ||
						bids.Status.CurrentState == longhorn.BackingImageStateReadyForTransfer ||
						bids.Status.CurrentState == longhorn.BackingImageStateReady
				if fileProcessingStarted || bids.Status.CurrentState == longhorn.BackingImageStateUnknown {
					log.Errorf("Backing image data source was state %v but the pod became not ready, the state will be updated to %v, message: %v", bids.Status.CurrentState, longhorn.BackingImageStateFailed, podNotReadyMessage)
					bids.Status.Message = podNotReadyMessage
					bids.Status.CurrentState = longhorn.BackingImageStateFailed
				}
			}
		}
		if c.isMonitoring(bids.Name) {
			c.stopMonitoring(bids.Name)
		}
	}

	if bids.Status.CurrentState == longhorn.BackingImageStateFailed ||
		bids.Status.CurrentState == longhorn.BackingImageStateFailedAndCleanUp {
		if err := c.cleanup(bids); err != nil {
			return err
		}
	}
	if pod == nil {
		// To avoid restarting backing image data source pod (for file preparation) too quickly or too frequently,
		// Longhorn will leave failed backing image data source alone if it is still in the backoff period.
		// If the backoff period pass, Longhorn will recreate the pod and increase the Backoff period for the next possible failure.
		isValidTypeForRetry := bids.Spec.SourceType == longhorn.BackingImageDataSourceTypeDownload || bids.Spec.SourceType == longhorn.BackingImageDataSourceTypeExportFromVolume
		isInBackoffWindow := true
		if !newBackingImageDataSource && isValidTypeForRetry {
			if !c.backoff.IsInBackOffSinceUpdate(bids.Name, time.Now()) {
				isInBackoffWindow = false
				log.Infof("Preparing to recreate pod for image data source %v since the backoff window is already passed", bids.Name)
			} else {
				log.Debugf("Failed backing image data source %v is still in the backoff window, Longhorn cannot recreate pod for it", bids.Name)
			}
		}

		if newBackingImageDataSource ||
			(isValidTypeForRetry && !isInBackoffWindow) {
			if err := c.handleAttachmentTicketCreation(bids); err != nil {
				return err
			}
			// For recovering the backing image exported from volumes, the controller needs to update the state regardless of the pod being created immediately.
			// Otherwise, the backing image data source will stay in state failed/unknown then the volume controller won't do auto attachment.
			bids.Status.CurrentState = ""
			bids.Status.Message = ""
			bids.Status.Progress = 0
			bids.Status.Checksum = ""
			if err := c.createBackingImageDataSourcePod(bids); err != nil {
				return err
			}
			// The backoff entry will be cleaned up when the monitor finds the file becoming ready.
			c.backoff.Next(bids.Name, time.Now())
		}
	}

	return nil
}

// handleAttachmentTicketDeletion check and delete attachment so that the source volume is detached if needed
func (c *BackingImageDataSourceController) handleAttachmentTicketDeletion(bids *longhorn.BackingImageDataSource) (err error) {
	if bids.Spec.SourceType != longhorn.BackingImageDataSourceTypeExportFromVolume {
		return nil
	}

	volumeName := bids.Spec.Parameters[longhorn.DataSourceTypeExportFromVolumeParameterVolumeName]
	va, err := c.ds.GetLHVolumeAttachmentByVolumeName(volumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	attachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeBackingImageDataSourceController, bids.Name)

	if _, ok := va.Spec.AttachmentTickets[attachmentTicketID]; ok {
		delete(va.Spec.AttachmentTickets, attachmentTicketID)
		if _, err = c.ds.UpdateLHVolumeAttachment(va); err != nil {
			return err
		}
	}

	return nil
}

// handleAttachmentTicketCreation check and create attachment so that the source volume is attached if needed
func (c *BackingImageDataSourceController) handleAttachmentTicketCreation(bids *longhorn.BackingImageDataSource) (err error) {
	if bids.Spec.SourceType != longhorn.BackingImageDataSourceTypeExportFromVolume {
		return nil
	}

	volumeName := bids.Spec.Parameters[longhorn.DataSourceTypeExportFromVolumeParameterVolumeName]
	vol, err := c.ds.GetVolume(volumeName)
	if err != nil {
		return err
	}

	va, err := c.ds.GetLHVolumeAttachmentByVolumeName(vol.Name)
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

	attachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeBackingImageDataSourceController, bids.Name)
	createOrUpdateAttachmentTicket(va, attachmentTicketID, vol.Status.OwnerID, longhorn.AnyValue, longhorn.AttacherTypeBackingImageDataSourceController)

	return nil
}

func (c *BackingImageDataSourceController) createBackingImageDataSourcePod(bids *longhorn.BackingImageDataSource) (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to create backing image data source pod")
	}()

	log := getLoggerForBackingImageDataSource(c.logger, bids)

	log.Info("Creating backing image data source pod")

	podManifest, err := c.generateBackingImageDataSourcePodManifest(bids)
	if err != nil {
		return err
	}
	if _, err := c.ds.CreatePod(podManifest); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (c *BackingImageDataSourceController) generateBackingImageDataSourcePodManifest(bids *longhorn.BackingImageDataSource) (*corev1.Pod, error) {
	nodeSelector, err := c.ds.GetSettingSystemManagedComponentsNodeSelector()
	if err != nil {
		return nil, err
	}

	tolerations, err := c.ds.GetSettingTaintToleration()
	if err != nil {
		return nil, err
	}
	tolerationsByte, err := json.Marshal(tolerations)
	if err != nil {
		return nil, err
	}

	priorityClass, err := c.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return nil, err
	}

	imagePullPolicy, err := c.ds.GetSettingImagePullPolicy()
	if err != nil {
		return nil, err
	}

	bi, err := c.ds.GetBackingImage(bids.Name)
	if err != nil {
		return nil, err
	}
	if bi.Status.UUID == "" {
		return nil, fmt.Errorf("failed to start backing image data source pod since the backing image UUID is not set")
	}

	cmd := []string{
		"backing-image-manager", "--debug",
		"data-source",
		"--listen", fmt.Sprintf("%s:%d", "0.0.0.0", engineapi.BackingImageDataSourceDefaultPort),
		"--sync-listen", fmt.Sprintf("%s:%d", "0.0.0.0", engineapi.BackingImageSyncServerDefaultPort),
		"--name", bids.Name,
		"--uuid", bids.Spec.UUID,
		"--source-type", string(bids.Spec.SourceType),
	}

	if err := c.prepareRunningParameters(bids); err != nil {
		return nil, err
	}
	for key, value := range bids.Status.RunningParameters {
		cmd = append(cmd, "--parameters", fmt.Sprintf("%s=%s", key, value))
	}
	if bids.Spec.Checksum != "" {
		cmd = append(cmd, "--checksum", bids.Spec.Checksum)
	}

	privileged := true
	podSpec := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            types.GetBackingImageDataSourcePodName(bids.Name),
			Namespace:       c.namespace,
			OwnerReferences: datastore.GetOwnerReferencesForBackingImageDataSource(bids),
			Labels:          types.GetBackingImageDataSourceLabels(bids.Name, bids.Spec.NodeID, bids.Spec.DiskUUID),
			Annotations:     map[string]string{types.GetLonghornLabelKey(types.LastAppliedTolerationAnnotationKeySuffix): string(tolerationsByte)},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: c.serviceAccount,
			Tolerations:        util.GetDistinctTolerations(tolerations),
			NodeSelector:       nodeSelector,
			PriorityClassName:  priorityClass.Value,
			Containers: []corev1.Container{
				{
					Name:            BackingImageDataSourcePodContainerName,
					Image:           c.bimImageName,
					ImagePullPolicy: imagePullPolicy,
					Command:         cmd,
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.FromInt(engineapi.BackingImageDataSourceDefaultPort),
							},
						},
						InitialDelaySeconds: datastore.PodProbeInitialDelay,
						PeriodSeconds:       datastore.PodProbePeriodSeconds,
						TimeoutSeconds:      datastore.PodProbeTimeoutSeconds,
						FailureThreshold:    datastore.PodLivenessProbeFailureThreshold,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "disk-path",
							MountPath: bimtypes.DiskPathInContainer,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name: types.EnvPodIP,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "status.podIP",
								},
							},
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "disk-path",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: bids.Spec.DiskPath,
						},
					},
				},
			},
			NodeName:      bids.Spec.NodeID,
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	registrySecretSetting, err := c.ds.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return nil, err
	}
	if registrySecretSetting.Value != "" {
		podSpec.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: registrySecretSetting.Value,
			},
		}
	}

	storageNetwork, err := c.ds.GetSetting(types.SettingNameStorageNetwork)
	if err != nil {
		return nil, err
	}

	nadAnnot := string(types.CNIAnnotationNetworks)
	if storageNetwork.Value != types.CniNetworkNone {
		podSpec.Annotations[nadAnnot] = types.CreateCniAnnotationFromSetting(storageNetwork)
	}

	return podSpec, nil
}

func (c *BackingImageDataSourceController) prepareRunningParameters(bids *longhorn.BackingImageDataSource) error {
	bids.Status.RunningParameters = bids.Spec.Parameters
	if bids.Spec.SourceType != longhorn.BackingImageDataSourceTypeExportFromVolume {
		return nil
	}

	volumeName := bids.Spec.Parameters[longhorn.DataSourceTypeExportFromVolumeParameterVolumeName]
	v, err := c.ds.GetVolume(volumeName)
	if err != nil {
		return err
	}
	if v.Status.State != longhorn.VolumeStateAttached {
		return fmt.Errorf("need to wait for volume %v attached before preparing parameters", volumeName)
	}
	bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterVolumeSize] = strconv.FormatInt(v.Spec.Size, 10)

	e, err := c.ds.GetVolumeCurrentEngine(volumeName)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("no engine for source volume %v before preparing parameters", volumeName)
	}
	if len(e.Status.ReplicaModeMap) == 0 || len(e.Status.CurrentReplicaAddressMap) == 0 {
		return fmt.Errorf("the current engine %v is not ready for backing image exporting", e.Name)
	}

	newSnapshotRequired := true
	if bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterSnapshotName] != "" {
		if _, ok := e.Status.Snapshots[bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterSnapshotName]]; ok {
			newSnapshotRequired = false
		}
	}
	if newSnapshotRequired {
		engineClientProxy, err := c.getEngineClientProxy(e)
		if err != nil {
			return err
		}
		defer engineClientProxy.Close()

		snapLabels := map[string]string{types.GetLonghornLabelKey(types.LonghornLabelSnapshotForExportingBackingImage): bids.Name}
		snapshotName, err := engineClientProxy.SnapshotCreate(e, bids.Name+"-"+util.RandomID(), snapLabels)
		if err != nil {
			return err
		}
		bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterSnapshotName] = snapshotName
	}

	for rName, mode := range e.Status.ReplicaModeMap {
		if mode != longhorn.ReplicaModeRW {
			continue
		}
		r, err := c.ds.GetReplica(rName)
		if err != nil {
			return err
		}
		if r.Status.CurrentState != longhorn.InstanceStateRunning {
			continue
		}
		rAddress := e.Status.CurrentReplicaAddressMap[rName]
		if rAddress == "" || rAddress != fmt.Sprintf("%s:%d", r.Status.StorageIP, r.Status.Port) {
			continue
		}
		bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterSenderAddress] = rAddress
	}
	if bids.Status.RunningParameters[longhorn.DataSourceTypeExportFromVolumeParameterSenderAddress] == "" {
		return fmt.Errorf("failed to get an available replica from volume %v during backing image %v exporting", v.Name, bids.Name)
	}

	return nil
}

func (c *BackingImageDataSourceController) enqueueBackingImageDataSource(backingImageDataSource interface{}) {
	key, err := controller.KeyFunc(backingImageDataSource)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to get key for object %#v: %v", backingImageDataSource, err))
		return
	}

	c.queue.Add(key)
}

func isBackingImageDataSourcePod(obj interface{}) bool {
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

	return pod.Labels[types.GetLonghornLabelComponentKey()] == types.LonghornLabelBackingImageDataSource
}

func (c *BackingImageDataSourceController) enqueueForBackingImage(obj interface{}) {
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

	backingImageDataSource, err := c.ds.GetBackingImageDataSource(backingImage.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		utilruntime.HandleError(fmt.Errorf("failed to get backing image data source %v: %v ", backingImage.Name, err))
		return
	}
	c.enqueueBackingImageDataSource(backingImageDataSource)
}

func (c *BackingImageDataSourceController) enqueueForVolume(obj interface{}) {
	volume, ok := obj.(*longhorn.Volume)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		volume, ok = deletedState.Obj.(*longhorn.Volume)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	bidsMap, err := c.ds.ListBackingImageDataSourcesExportingFromVolume(volume.Name)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list backing image data source based on volume %v: %v ", volume.Name, err))
		return
	}
	for _, bids := range bidsMap {
		c.enqueueBackingImageDataSource(bids)
	}
}

func (c *BackingImageDataSourceController) enqueueForLonghornNode(obj interface{}) {
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

	node, err := c.ds.GetNode(node.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// there is no Longhorn node created for the Kubernetes
			// node (e.g. controller/etcd node). Skip it
			return
		}
		utilruntime.HandleError(fmt.Errorf("failed to get node %v: %v ", node.Name, err))
		return
	}

	bidss, err := c.ds.ListBackingImageDataSourcesByNode(node.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.WithField("node", node.Name).Warn("Failed to list backing image data sources for a node, may be deleted")
			return
		}
		utilruntime.HandleError(fmt.Errorf("failed to get backing image data source: %v", err))
		return
	}

	for _, bids := range bidss {
		c.enqueueBackingImageDataSource(bids)
	}
}

func (c *BackingImageDataSourceController) enqueueForBackingImageDataSourcePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("received unexpected obj: %#v", obj))
			return
		}

		// use the last known state, to enqueue, dependent objects
		pod, ok = deletedState.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("DeletedFinalStateUnknown contained invalid object: %#v", deletedState.Obj))
			return
		}
	}

	bidsName := pod.Labels[types.GetLonghornLabelKey(types.LonghornLabelBackingImageDataSource)]
	if bidsName == "" {
		return
	}
	bids, err := c.ds.GetBackingImageDataSource(bidsName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.WithField("pod", pod.Name).Warnf("Failed to find backing image data source %v for pod, may be deleted", bidsName)
			return
		}
		utilruntime.HandleError(fmt.Errorf("failed to get backing image data source: %v", err))
		return
	}
	c.enqueueBackingImageDataSource(bids)
}

func (c *BackingImageDataSourceController) isMonitoring(bidsName string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	_, ok := c.monitorMap[bidsName]
	return ok
}

func (c *BackingImageDataSourceController) stopMonitoring(bidsName string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	log := c.logger.WithField("backingImageDataSource", bidsName)
	stopCh, ok := c.monitorMap[bidsName]
	if !ok {
		return
	}
	log.Info("Stopping monitoring")
	close(stopCh)
	delete(c.monitorMap, bidsName)
}

func (c *BackingImageDataSourceController) startMonitoring(bids *longhorn.BackingImageDataSource) {
	log := getLoggerForBackingImageDataSource(c.logger, bids)

	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.monitorMap[bids.Name]; ok {
		return
	}

	if bids.Status.IP == "" {
		log.Error("Failed to get backing image data source pod IP before launching the monitor")
		return
	}

	stopCh := make(chan struct{}, 1)
	m := &BackingImageDataSourceMonitor{
		Name:         bids.Name,
		client:       engineapi.NewBackingImageDataSourceClient(bids.Status.IP),
		stopCh:       stopCh,
		controllerID: c.controllerID,
		log:          log,
		ds:           c.ds,
		backoff:      c.backoff,
	}
	c.monitorMap[bids.Name] = stopCh

	log.Info("Starting monitoring")

	// TODO: refactor this monitor. ref: https://github.com/longhorn/longhorn/issues/2441
	go wait.Until(m.sync, engineapi.BackingImageDataSourcePollInterval, stopCh)
	go func() {
		<-m.stopCh
		c.stopMonitoring(bids.Name)
	}()
}

func (m *BackingImageDataSourceMonitor) sync() {
	var syncErr error
	defer func() {
		if syncErr != nil {
			m.retryCount++
			if m.retryCount == engineapi.MaxMonitorRetryCount {
				m.stopCh <- struct{}{}
				m.log.Warnf("Stopped monitoring since monitor %v sync reaches the max retry count %v", m.Name, engineapi.MaxMonitorRetryCount)
				return
			}
		} else {
			m.retryCount = 0
		}

	}()

	bids, err := m.ds.GetBackingImageDataSource(m.Name)
	if err != nil {
		if datastore.ErrorIsNotFound(err) {
			m.stopCh <- struct{}{}
			m.log.Warnf("Stopped monitoring since backing image data source %v is not found", m.Name)
			return
		}
		syncErr = errors.Wrapf(err, "failed to get backing image data source %v during monitor sync", m.Name)
		m.log.Error(syncErr)
		return
	}
	if bids.Status.OwnerID != m.controllerID {
		m.stopCh <- struct{}{}
		m.log.Warnf("Stopped monitoring since backing image data source %v owner %v is not the same as monitor current controller %v", m.Name, bids.Status.OwnerID, m.controllerID)
		return
	}
	if bids.Status.IP == "" {
		m.log.Warnf("Stopped monitoring since backing image data source %v current IP is empty", m.Name)
		return
	}

	fileInfo, err := m.client.Get()
	if err != nil {
		syncErr = errors.Wrapf(err, "failed to get %v info from backing image data source server", m.Name)
		m.log.Error(syncErr)
		return
	}

	if fileInfo.State == string(longhorn.BackingImageStateFailed) && fileInfo.Message != "" {
		m.log.Errorf("Failed to prepare the file for backing image data source, error message: %v", fileInfo.Message)
	}

	existingBIDS := bids.DeepCopy()
	bids.Status.CurrentState = longhorn.BackingImageState(fileInfo.State)
	bids.Status.Size = fileInfo.Size
	bids.Status.Progress = fileInfo.Progress
	bids.Status.Checksum = fileInfo.CurrentChecksum
	bids.Status.Message = fileInfo.Message
	if !reflect.DeepEqual(bids.Status, existingBIDS.Status) {
		if _, err := m.ds.UpdateBackingImageDataSourceStatus(bids); err != nil {
			syncErr = errors.Wrapf(err, "failed to get %v info from backing image data source server", m.Name)
			m.log.Error(syncErr)
			return
		}
	}

	if bids.Status.CurrentState == longhorn.BackingImageStateReady || bids.Status.CurrentState == longhorn.BackingImageStateReadyForTransfer {
		m.backoff.DeleteEntry(bids.Name)
	}
}

func (c *BackingImageDataSourceController) isResponsibleFor(bids *longhorn.BackingImageDataSource) bool {
	return isControllerResponsibleFor(c.controllerID, c.ds, bids.Name, bids.Spec.NodeID, bids.Status.OwnerID)
}

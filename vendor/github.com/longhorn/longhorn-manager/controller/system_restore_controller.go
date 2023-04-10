package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SystemRestoreControllerName = "longhorn-system-restore"

	RestoreJobBackoffLimit = 3

	SystemRestoreErrJobCreate     = "failed to create system restore Job"
	SystemRestoreMsgJobCreatedFmt = "Created system restore Job %v"
	SystemRestoreMsgDeleting      = "Deleting SystemRestore"
)

type systemRestoreRecordType string

const (
	systemRestoreRecordTypeError  = systemRestoreRecordType("error")
	systemRestoreRecordTypeNone   = systemRestoreRecordType("")
	systemRestoreRecordTypeNormal = systemRestoreRecordType("normal")
)

type systemRestoreRecord struct {
	nextState longhorn.SystemRestoreState

	recordType systemRestoreRecordType
	message    string
	reason     string
}

type SystemRestoreController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewSystemRestoreController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	namespace string,
	controllerID string) *SystemRestoreController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	c := &SystemRestoreController{
		baseController: newBaseController(SystemRestoreControllerName, logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: SystemRestoreControllerName + "-controller"}),
	}

	ds.SystemRestoreInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueSystemRestore,
		UpdateFunc: func(old, cur interface{}) { c.enqueueSystemRestore(cur) },
		DeleteFunc: c.enqueueSystemRestore,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.SystemRestoreInformer.HasSynced)

	return c
}

func (c *SystemRestoreController) enqueueSystemRestore(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	c.queue.Add(key)
}

func (c *SystemRestoreController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting Longhorn SystemRestore controller")
	defer c.logger.Info("Shut down Longhorn SystemRestore controller")

	if !cache.WaitForNamedCacheSync(c.name, stopCh, c.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (c *SystemRestoreController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *SystemRestoreController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncSystemRestore(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *SystemRestoreController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	log := c.logger.WithField("systemRestore", key)

	if c.queue.NumRequeues(key) < maxRetries {
		log.WithError(err).Warn("Failed to sync SystemRestore")

		c.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	log.WithError(err).Warnf("Dropping Longhorn SystemRestore %v out of the queue", key)
	c.queue.Forget(key)
}

func getLoggerForSystemRestore(logger logrus.FieldLogger, systemRestore *longhorn.SystemRestore) *logrus.Entry {
	return logger.WithField("systemRestore", systemRestore.Name)
}

func (c *SystemRestoreController) LogErrorState(record *systemRestoreRecord, systemRestore *longhorn.SystemRestore, log logrus.FieldLogger) {
	log.Error(record.message)
	c.eventRecorder.Eventf(systemRestore, corev1.EventTypeWarning, constant.EventReasonFailed, util.CapitalizeFirstLetter(record.message))
}

func (c *SystemRestoreController) LogNormalState(record *systemRestoreRecord, systemRestore *longhorn.SystemRestore, log logrus.FieldLogger) {
	log.Info(record.message)
	c.eventRecorder.Eventf(systemRestore, corev1.EventTypeNormal, record.reason, record.message)
}

func (c *SystemRestoreController) syncSystemRestore(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync SystemRestore %v", c.name, key)
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

func (c *SystemRestoreController) reconcile(name string, backupTargetClient engineapi.SystemBackupOperationInterface) (err error) {
	systemRestore, err := c.ds.GetSystemRestore(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	log := getLoggerForSystemRestore(c.logger, systemRestore)

	if !c.isResponsibleFor(systemRestore) {
		return nil
	}

	if systemRestore.Status.OwnerID != c.controllerID {
		systemRestore.Status.OwnerID = c.controllerID
		systemRestore, err = c.ds.UpdateSystemRestoreStatus(systemRestore)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Picked up by SystemRestore Controller %v", c.controllerID)
	}

	record := &systemRestoreRecord{}
	existingSystemRestore := systemRestore.DeepCopy()
	defer func() {
		c.handleStatusUpdate(record, systemRestore, existingSystemRestore, err, log)
	}()

	if !systemRestore.DeletionTimestamp.IsZero() && systemRestore.Status.State != longhorn.SystemRestoreStateDeleting {
		c.updateSystemRestoreRecord(record,
			systemRestoreRecordTypeNormal, longhorn.SystemRestoreStateDeleting,
			constant.EventReasonDeleting, SystemRestoreMsgDeleting,
		)
		return
	}

	switch systemRestore.Status.State {
	case longhorn.SystemRestoreStateCompleted, longhorn.SystemRestoreStateError:
		_, err = c.ds.RemoveSystemRestoreLabel(systemRestore)
		return err

	case longhorn.SystemRestoreStateDeleting:
		return c.cleanupSystemRestore(systemRestore)

	case longhorn.SystemRestoreStateNone:
		_, err = c.ds.GetJob(systemRestore.Name)
		if err == nil {
			return nil
		}

		if !apierrors.IsNotFound(err) {
			return err
		}

		job, err := c.CreateSystemRestoreJob(systemRestore, backupTargetClient)
		if err != nil {
			err = errors.Wrapf(err, SystemRestoreErrJobCreate)
			c.updateSystemRestoreRecord(record,
				systemRestoreRecordTypeError, longhorn.SystemRestoreStateError,
				fmt.Sprintf(constant.EventReasonFailedCreatingFmt, types.KubernetesKindJob, "for "+systemRestore.Name),
				err.Error(),
			)
			return nil
		}

		c.updateSystemRestoreRecord(record,
			systemRestoreRecordTypeNormal, longhorn.SystemRestoreStatePending,
			constant.EventReasonCreated, fmt.Sprintf(SystemRestoreMsgJobCreatedFmt, job.Name),
		)
		return nil

	default:
		// The system-rollout controller will handle the rest of the state
		// change once the system starts restoring.
	}

	return nil
}

func (c *SystemRestoreController) handleStatusUpdate(record *systemRestoreRecord, systemRestore *longhorn.SystemRestore, existingSystemRestore *longhorn.SystemRestore, err error, log logrus.FieldLogger) {
	switch record.recordType {
	case systemRestoreRecordTypeError:
		c.recordErrorState(record, systemRestore)
	case systemRestoreRecordTypeNormal:
		c.recordNormalState(record, systemRestore)
	default:
		return
	}

	if !reflect.DeepEqual(existingSystemRestore.Status, systemRestore.Status) {
		updated, err := c.ds.UpdateSystemRestoreStatus(systemRestore)
		if err != nil {
			log.WithError(err).Debugf("Requeue %v due to error", systemRestore.Name)
			c.enqueueSystemRestore(systemRestore)
			err = nil
			return
		}
		systemRestore = updated
	}

	switch record.recordType {
	case systemRestoreRecordTypeError:
		c.LogErrorState(record, systemRestore, log)
	case systemRestoreRecordTypeNormal:
		c.LogNormalState(record, systemRestore, log)
	default:
		return
	}

	if systemRestore.Status.State != existingSystemRestore.Status.State {
		log.Infof(SystemRolloutMsgRequeueNextPhaseFmt, systemRestore.Status.State)
	}
}

func (c *SystemRestoreController) recordErrorState(record *systemRestoreRecord, systemRestore *longhorn.SystemRestore) {
	systemRestore.Status.State = longhorn.SystemRestoreStateError
	systemRestore.Status.Conditions = types.SetCondition(
		systemRestore.Status.Conditions,
		longhorn.SystemBackupConditionTypeError,
		longhorn.ConditionStatusTrue,
		record.reason,
		record.message,
	)
}

func (c *SystemRestoreController) recordNormalState(record *systemRestoreRecord, systemRestore *longhorn.SystemRestore) {
	systemRestore.Status.State = record.nextState
}

func (c *SystemRestoreController) updateSystemRestoreRecord(record *systemRestoreRecord, recordType systemRestoreRecordType, nextState longhorn.SystemRestoreState, reason, message string) {
	record.recordType = recordType
	record.nextState = nextState
	record.reason = reason
	record.message = message
}

func (c *SystemRestoreController) CreateSystemRestoreJob(systemRestore *longhorn.SystemRestore, backupTargetClient engineapi.SystemBackupOperationInterface) (*batchv1.Job, error) {
	systemBackup, err := c.ds.GetSystemBackupRO(systemRestore.Spec.SystemBackup)
	if err != nil {
		return nil, err
	}

	cfg, err := backupTargetClient.GetSystemBackupConfig(systemBackup.Name, systemBackup.Status.Version)
	if err != nil {
		return nil, err
	}

	serviceAccountName, err := c.getLonghornServiceAccountName()
	if err != nil {
		return nil, err
	}

	return c.ds.CreateJob(c.newSystemRestoreJob(systemRestore, c.namespace, cfg.ManagerImage, serviceAccountName))
}

func (c *SystemRestoreController) newSystemRestoreJob(systemRestore *longhorn.SystemRestore, namespace, managerImage, serviceAccount string) *batchv1.Job {
	backoffLimit := int32(RestoreJobBackoffLimit)

	// This is required for the NFS mount to access the backup store
	privileged := true

	cmd := []string{
		"longhorn-manager", "system-rollout", systemRestore.Name,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            getSystemRolloutName(systemRestore.Name),
			Namespace:       namespace,
			OwnerReferences: datastore.GetOwnerReferencesForSystemRestore(systemRestore),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: systemRestore.Name,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers: []corev1.Container{
						{
							Name:    getSystemRolloutName(systemRestore.Name),
							Image:   managerImage,
							Command: cmd,
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "NODE_NAME",
									Value: c.controllerID,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "engine",
									MountPath: types.EngineBinaryDirectoryOnHost,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "engine",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: types.EngineBinaryDirectoryOnHost,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					NodeSelector: map[string]string{
						corev1.LabelHostname: c.controllerID,
					},
				},
			},
		},
	}

	return job
}

func (c *SystemRestoreController) getLonghornServiceAccountName() (string, error) {
	managerPods, err := c.ds.ListManagerPods()
	if err != nil {
		return "", err
	}

	for _, pod := range managerPods {
		return pod.Spec.ServiceAccountName, nil
	}
	return "", errors.Errorf("failed to find service account from manager pods")
}

func (c *SystemRestoreController) isResponsibleFor(systemRestore *longhorn.SystemRestore) bool {
	return isControllerResponsibleFor(c.controllerID, c.ds, systemRestore.Name, "", systemRestore.Status.OwnerID)
}

func (c *SystemRestoreController) cleanupSystemRestore(systemRestore *longhorn.SystemRestore) (err error) {
	log := getLoggerForSystemRestore(c.logger, systemRestore)

	defer func() {
		if err == nil {
			return
		}

		log.WithError(err).Error("Failed to delete SystemRestore")
		systemRestore.Status.Conditions = types.SetCondition(systemRestore.Status.Conditions,
			longhorn.SystemRestoreConditionTypeError, longhorn.ConditionStatusTrue, "", err.Error())
	}()

	systemRolloutName := getSystemRolloutName(systemRestore.Name)
	_, err = c.ds.GetJob(systemRolloutName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return
	}

	log.WithField("job", systemRolloutName).Debug("Deleting job")
	return c.ds.DeleteJob(systemRolloutName)
}

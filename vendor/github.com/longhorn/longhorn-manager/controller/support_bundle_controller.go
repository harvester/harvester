package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	api "k8s.io/kubernetes/pkg/apis/core"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SupportBundleServiceAccount = "longhorn-support-bundle"

	SupportBundleRequeueDelay = time.Second

	SupportBundleMsgRequeueOnConflictFmt = "Requeue %v due to conflict"
	SupportBundleMsgRequeueNextPhaseFmt  = "Requeue %v for next phase: %v"

	SupportBundleMsgManagerPhase = "Support bundle manager updated phase: %v"

	SupportBundleMsgDeleting = "Deleting support bundle manager"
	SupportBundleMsgPurging  = "Purging failed support bundle"
)

type supportBundleRecordType string

const (
	supportBundleRecordNone   = supportBundleRecordType("")
	supportBundleRecordError  = supportBundleRecordType("error")
	supportBundleRecordNormal = supportBundleRecordType("normal")
)

type supportBundleRecord struct {
	nextState longhorn.SupportBundleState

	recordType supportBundleRecordType
	reason     string
	message    string
}

type SupportBundleController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID   string
	serviceAccount string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced

	httpClient rest.HTTPClient
}

func NewSupportBundleController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace, serviceAccount string) *SupportBundleController {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	c := &SupportBundleController{
		baseController: newBaseController("longhorn-support-bundle", logger),
		controllerID:   controllerID,

		namespace:      namespace,
		serviceAccount: serviceAccount,

		ds: ds,

		kubeClient: kubeClient,
		httpClient: &http.Client{Timeout: 30 * time.Second},

		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-support-bundle-controller"}),
	}

	ds.SupportBundleInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		UpdateFunc: func(old, cur interface{}) { c.enqueue(cur) },
		DeleteFunc: c.enqueue,
	}, 0)
	c.cacheSyncs = append(c.cacheSyncs, ds.OrphanInformer.HasSynced)

	return c
}

func (c *SupportBundleController) enqueue(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	c.queue.Add(key)
}

func (c *SupportBundleController) enqueueAfter(obj interface{}, delay time.Duration) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	c.queue.AddAfter(key, delay)
}

func (c *SupportBundleController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting Longhorn Support Bundle controller")
	defer c.logger.Info("Shut down Longhorn Support Bundle controller")

	if !cache.WaitForNamedCacheSync(c.name, stopCh, c.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (c *SupportBundleController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *SupportBundleController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncSupportBundle(key.(string))
	c.handleErr(err, key)

	return true
}

func (c *SupportBundleController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	log := c.logger.WithField("supportBundle", key)

	if c.queue.NumRequeues(key) < maxRetries {
		log.WithError(err).Warn("Error syncing Longhorn SupportBundle")

		c.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	log.WithError(err).Warn("Dropping Longhorn SupportBundle out of the queue")
	c.queue.Forget(key)
}

func (c *SupportBundleController) handleStatusUpdate(record *supportBundleRecord, supportBundle *longhorn.SupportBundle, existing *longhorn.SupportBundle, log logrus.FieldLogger) {
	var err error
	switch record.recordType {
	case supportBundleRecordError:
		c.recordErrorState(record, supportBundle, log)

		failedLimit, err := c.ds.GetSettingAsInt(types.SettingNameSupportBundleFailedHistoryLimit)
		if err != nil {
			log.WithError(err).Warnf("Failed to get %v setting for SupportBundle %v cleanup", types.SettingNameSupportBundleFailedHistoryLimit, supportBundle.Name)
			break
		}

		if failedLimit > 0 {
			break
		}

		c.updateSupportBundleRecord(record,
			supportBundleRecordNormal, longhorn.SupportBundleStatePurging,
			constant.EventReasonDeleting, SupportBundleMsgPurging,
		)
		fallthrough

	case supportBundleRecordNormal:
		c.recordNormalState(record, supportBundle, log)
	}

	isStatusChange := !reflect.DeepEqual(existing.Status, supportBundle.Status)
	if isStatusChange {
		supportBundle, err = c.ds.UpdateSupportBundleStatus(supportBundle)
		if apierrors.IsConflict(errors.Cause(err)) {
			log.WithError(err).Debugf(SupportBundleMsgRequeueOnConflictFmt, supportBundle.Name)
			c.enqueue(supportBundle)
		}

		if supportBundle.Status.State != existing.Status.State {
			log.Infof(SupportBundleMsgRequeueNextPhaseFmt, supportBundle.Name, supportBundle.Status.State)
		}
		return
	}

	switch supportBundle.Status.State {
	case longhorn.SupportBundleStateReady, longhorn.SupportBundleStateError:
		return
	}

	c.enqueueAfter(supportBundle, SupportBundleRequeueDelay)
}

func (c *SupportBundleController) updateSupportBundleRecord(record *supportBundleRecord, recordType supportBundleRecordType, nextState longhorn.SupportBundleState, reason, message string) error {
	record.recordType = recordType
	record.nextState = nextState
	record.reason = reason
	record.message = message
	return nil
}

func getLoggerForSupportBundle(logger logrus.FieldLogger, name string) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"supportBundle": name,
		},
	)
}

func (c *SupportBundleController) syncSupportBundle(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync SupportBundle %v", c.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	if namespace != c.namespace {
		return nil
	}

	return c.reconcile(name)
}

func (c *SupportBundleController) reconcile(name string) (err error) {
	supportBundle, err := c.ds.GetSupportBundle(name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	log := getLoggerForSupportBundle(c.logger, supportBundle.Name)

	if !c.isResponsibleFor(supportBundle) {
		return nil
	}

	if supportBundle.Status.OwnerID != c.controllerID {
		supportBundle.Status.OwnerID = c.controllerID
		supportBundle, err = c.ds.UpdateSupportBundleStatus(supportBundle)
		if err != nil {
			// we don't mind others coming first
			if apierrors.IsConflict(errors.Cause(err)) {
				return nil
			}
			return err
		}
		log.Infof("Picked up by SupportBundle Controller %v", c.controllerID)
	}

	record := &supportBundleRecord{}
	existingSupportBundle := supportBundle.DeepCopy()
	defer c.handleStatusUpdate(record, supportBundle, existingSupportBundle, log)

	if !supportBundle.DeletionTimestamp.IsZero() && supportBundle.Status.State != longhorn.SupportBundleStateDeleting {
		return c.updateSupportBundleRecord(record,
			supportBundleRecordNormal, longhorn.SupportBundleStateDeleting,
			constant.EventReasonDeleting, SupportBundleMsgDeleting,
		)
	}

	switch supportBundle.Status.State {
	case longhorn.SupportBundleStateNone:
		var supportBundleManagerImage string
		supportBundleManagerImage, err = c.ds.GetSettingValueExisted(types.SettingNameSupportBundleManagerImage)
		if err != nil {
			return c.updateSupportBundleRecord(record,
				supportBundleRecordError, longhorn.SupportBundleStateError,
				constant.EventReasonFailedStarting, err.Error(),
			)
		}

		supportBundle.Status.Image = supportBundleManagerImage
		c.updateSupportBundleRecord(record,
			supportBundleRecordNormal, longhorn.SupportBundleStateStarted,
			constant.EventReasonStart, fmt.Sprintf(longhorn.SupportBundleMsgInitFmt, supportBundle.Name),
		)

	case longhorn.SupportBundleStateStarted:
		var supportBundleManagerDeployment *appsv1.Deployment
		supportBundleManagerDeployment, err = c.ds.GetDeployment(GetSupportBundleManagerName(supportBundle))
		if err != nil {
			if apierrors.IsNotFound(err) {
				supportBundleManagerDeployment, err = c.createSupportBundleManagerDeployment(supportBundle)
			}

			if err != nil {
				return c.updateSupportBundleRecord(record,
					supportBundleRecordError, longhorn.SupportBundleStateError,
					constant.EventReasonFailedStarting, err.Error(),
				)
			}
		}

		_, err = c.getSupportBundleManager(supportBundle, log)
		if err != nil {
			logrus.WithError(err).Debug("Waiting for support bundle manager to start")
			return nil
		}

		c.updateSupportBundleRecord(record,
			supportBundleRecordNormal, longhorn.SupportBundleStateGenerating,
			constant.EventReasonCreate, fmt.Sprintf(longhorn.SupportBundleMsgCreateManagerFmt, supportBundleManagerDeployment.Name),
		)

	case longhorn.SupportBundleStateGenerating:
		var supportBundleManager *SupportBundleManager
		supportBundleManager, err = c.getSupportBundleManager(supportBundle, log)
		if err != nil {
			return c.updateSupportBundleRecord(record,
				supportBundleRecordError, longhorn.SupportBundleStateError,
				fmt.Sprintf(constant.EventReasonFailedCreatingFmt, types.LonghornKindSupportBundle, supportBundle.Name),
				err.Error(),
			)
		}

		c.recordManagerState(supportBundleManager, supportBundle, log)

		if supportBundle.Status.Progress != 100 {
			log.WithError(err).Debug("Waiting for support bundle manager to finish")
			return nil
		}

		message := fmt.Sprintf(longhorn.SupportBundleMsgGeneratedFmt,
			supportBundle.Status.Filename,
			fmt.Sprintf(types.SupportBundleURLDownloadFmt, supportBundleManager.podIP, types.SupportBundleURLPort),
		)
		c.updateSupportBundleRecord(record,
			supportBundleRecordNormal, longhorn.SupportBundleStateReady,
			constant.EventReasonCreate, message,
		)

	case longhorn.SupportBundleStateDeleting:
		err := c.cleanupSupportBundle(supportBundle)
		if err != nil {
			return c.updateSupportBundleRecord(record,
				supportBundleRecordError, longhorn.SupportBundleStateError,
				constant.EventReasonFailedDeleting, err.Error(),
			)
		}

		err = c.ds.RemoveFinalizerForSupportBundle(supportBundle)
		if err != nil {
			return c.updateSupportBundleRecord(record,
				supportBundleRecordError, longhorn.SupportBundleStateError,
				constant.EventReasonFailedDeleting, err.Error(),
			)
		}

		log.Info("Deleted SupportBundle")

	case longhorn.SupportBundleStatePurging:
		supportBundles, err := c.ds.ListSupportBundles()
		if err != nil {
			return c.updateSupportBundleRecord(record,
				supportBundleRecordError, longhorn.SupportBundleStateError,
				constant.EventReasonFailedDeleting, err.Error(),
			)
		}

		for _, otherSupportBundle := range supportBundles {
			if supportBundle.Status.OwnerID != c.controllerID {
				continue
			}
			if otherSupportBundle.Name == supportBundle.Name {
				continue
			}
			if otherSupportBundle.Status.State == longhorn.SupportBundleStatePurging {
				continue
			}
			if otherSupportBundle.Status.State == longhorn.SupportBundleStateDeleting {
				continue
			}
			if otherSupportBundle.Status.State != longhorn.SupportBundleStateError {
				continue
			}

			otherSupportBundle.Status.State = longhorn.SupportBundleStatePurging
			_, err = c.ds.UpdateSupportBundleStatus(otherSupportBundle)
			if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
				log.WithError(err).Warnf("failed to update SupportBundle %v to state %v", otherSupportBundle.Name, longhorn.SupportBundleStatePurging)
			}
		}

		err = c.ds.DeleteSupportBundle(supportBundle.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			log.WithError(err).Warn("Failed to purge SupportBundle")
		}
	}

	return nil
}

type SupportBundleManagerPhase string

type SupportBundleManagerStatus struct {
	Phase        SupportBundleManagerPhase
	Error        bool
	ErrorMessage string
	Progress     int
	FileName     string
	FileSize     int64
}

type SupportBundleManager struct {
	status *SupportBundleManagerStatus
	podIP  string
}

func (c *SupportBundleController) getSupportBundleManager(supportBundle *longhorn.SupportBundle, log logrus.FieldLogger) (*SupportBundleManager, error) {
	supportBundleManagerPod, err := c.ds.GetSupportBundleManagerPod(supportBundle)
	if err != nil {
		return nil, err
	}

	supportBundleManager := &SupportBundleManager{
		status: &SupportBundleManagerStatus{},
	}
	supportBundleManager.podIP, err = util.GetPodIP(supportBundleManagerPod)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(types.SupportBundleURLStatusFmt, supportBundleManager.podIP, types.SupportBundleURLPort)
	status, err := c.getSupportBundleStatusFromManager(url)
	if err != nil {
		return nil, err
	}
	supportBundleManager.status = status

	return supportBundleManager, nil
}

func (c *SupportBundleController) getSupportBundleStatusFromManager(url string) (*SupportBundleManagerStatus, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	status := &SupportBundleManagerStatus{}
	err = json.NewDecoder(resp.Body).Decode(status)
	if err != nil {
		return nil, err
	}

	return status, nil
}

func (c *SupportBundleController) recordErrorState(record *supportBundleRecord, supportBundle *longhorn.SupportBundle, log logrus.FieldLogger) {
	supportBundle.Status.State = longhorn.SupportBundleStateError
	supportBundle.Status.Conditions = types.SetCondition(
		supportBundle.Status.Conditions,
		longhorn.SupportBundleConditionTypeError,
		longhorn.ConditionStatusTrue,
		record.reason,
		record.message,
	)

	log.Error(record.message)
	c.eventRecorder.Eventf(supportBundle, corev1.EventTypeWarning, constant.EventReasonFailed, util.CapitalizeFirstLetter(record.message))
}

func (c *SupportBundleController) recordNormalState(record *supportBundleRecord, supportBundle *longhorn.SupportBundle, log logrus.FieldLogger) {
	supportBundle.Status.State = record.nextState

	log.Info(record.message)
	c.eventRecorder.Eventf(supportBundle, corev1.EventTypeNormal, record.reason, record.message)
}

func (c *SupportBundleController) recordManagerState(supportBundleManager *SupportBundleManager, supportBundle *longhorn.SupportBundle, log logrus.FieldLogger) {
	supportBundleManagerStatus := supportBundleManager.status
	supportBundle.Status.IP = supportBundleManager.podIP
	supportBundle.Status.Filename = supportBundleManagerStatus.FileName
	supportBundle.Status.Progress = supportBundleManagerStatus.Progress
	supportBundle.Status.Filesize = supportBundleManagerStatus.FileSize

	message := string(supportBundleManagerStatus.Phase)
	if types.GetCondition(supportBundle.Status.Conditions, longhorn.SupportBundleConditionTypeManager).Message == message {
		return
	}

	supportBundle.Status.Conditions = types.SetCondition(
		supportBundle.Status.Conditions,
		longhorn.SupportBundleConditionTypeManager,
		longhorn.ConditionStatusTrue,
		constant.EventReasonCreate,
		message,
	)

	c.eventRecorder.Eventf(supportBundle, corev1.EventTypeNormal, constant.EventReasonCreate, fmt.Sprintf(SupportBundleMsgManagerPhase, message))
	log.Debug(message)

}

func (c *SupportBundleController) isResponsibleFor(supportBundle *longhorn.SupportBundle) bool {
	return isControllerResponsibleFor(c.controllerID, c.ds, supportBundle.Name, supportBundle.Spec.NodeID, supportBundle.Status.OwnerID)
}

func (c *SupportBundleController) cleanupSupportBundle(supportBundle *longhorn.SupportBundle) error {
	supportBundleManagerName := GetSupportBundleManagerName(supportBundle)
	deployment, err := c.ds.GetDeployment(supportBundleManagerName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return c.ds.DeleteDeployment(deployment.Name)
}

func GetSupportBundleManagerName(supportBundle *longhorn.SupportBundle) string {
	return fmt.Sprintf("longhorn-support-bundle-manager-%v", supportBundle.Name)
}

func (c *SupportBundleController) createSupportBundleManagerDeployment(supportBundle *longhorn.SupportBundle) (*appsv1.Deployment, error) {
	nodeSelector, err := c.ds.GetSettingSystemManagedComponentsNodeSelector()
	if err != nil {
		return nil, err
	}

	imagePullPolicy, err := c.ds.GetSettingImagePullPolicy()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get system pods image pull policy before creating support bundle manager deployment")
	}

	priorityClassSetting, err := c.ds.GetSetting(types.SettingNamePriorityClass)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get priority class setting before creating support bundle manager deployment")
	}
	priorityClass := priorityClassSetting.Value

	registrySecretSetting, err := c.ds.GetSetting(types.SettingNameRegistrySecret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get registry secret setting before creating support bundle manager deployment")
	}
	registrySecret := registrySecretSetting.Value

	newSupportBundleManager, err := c.newSupportBundleManager(supportBundle, nodeSelector, imagePullPolicy, priorityClass, registrySecret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new support bundle manager")
	}
	return c.ds.CreateDeployment(newSupportBundleManager)
}

func (c *SupportBundleController) newSupportBundleManager(supportBundle *longhorn.SupportBundle, nodeSelector map[string]string,
	imagePullPolicy corev1.PullPolicy, priorityClass, registrySecret string) (*appsv1.Deployment, error) {

	tolerationSetting, err := c.ds.GetSetting(types.SettingNameTaintToleration)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %v setting", types.SettingNameTaintToleration)
	}
	tolerations, err := types.UnmarshalTolerations(tolerationSetting.Value)
	if err != nil {
		return nil, err
	}

	supportBundleManagerName := GetSupportBundleManagerName(supportBundle)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            supportBundleManagerName,
			Namespace:       c.namespace,
			Labels:          c.ds.GetSupportBundleManagerLabel(supportBundle),
			OwnerReferences: datastore.GetOwnerReferencesForSupportBundle(supportBundle),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": types.SupportBundleManagerApp},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: c.ds.GetSupportBundleManagerLabel(supportBundle),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: SupportBundleServiceAccount,
					Tolerations:        tolerations,
					NodeSelector:       nodeSelector,
					PriorityClassName:  priorityClass,
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           supportBundle.Status.Image,
							Args:            []string{"/usr/bin/support-bundle-kit", "manager"},
							ImagePullPolicy: corev1.PullPolicy(api.PullAlways),
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
									Name:  "SUPPORT_BUNDLE_TARGET_NAMESPACES",
									Value: c.namespace,
								},
								{
									Name:  "SUPPORT_BUNDLE_NAME",
									Value: supportBundle.Name,
								},
								{
									Name:  "SUPPORT_BUNDLE_DEBUG",
									Value: "true",
								},
								{
									Name: "SUPPORT_BUNDLE_MANAGER_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "SUPPORT_BUNDLE_IMAGE",
									Value: supportBundle.Status.Image,
								},
								{
									Name:  "SUPPORT_BUNDLE_IMAGE_PULL_POLICY",
									Value: string(api.PullAlways),
								},
								{
									Name:  "SUPPORT_BUNDLE_REGISTRY_SECRET",
									Value: string(registrySecret),
								},
								{
									Name:  "SUPPORT_BUNDLE_COLLECTOR",
									Value: "longhorn",
								},
								{
									Name:  "SUPPORT_BUNDLE_NODE_SELECTOR",
									Value: c.getNodeSelectorString(nodeSelector),
								},
								{
									Name:  "SUPPORT_BUNDLE_TAINT_TOLERATION",
									Value: c.getTaintTolerationString(tolerationSetting),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: types.SupportBundleURLPort,
								},
							},
						},
					},
				},
			},
		},
	}

	if registrySecret != "" {
		deployment.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: registrySecret,
			},
		}
	}

	return deployment, nil
}

func (c *SupportBundleController) getNodeSelectorString(nodeSelector map[string]string) string {
	list := make([]string, 0, len(nodeSelector))
	for k, v := range nodeSelector {
		list = append(list, k+"="+v)
	}
	return strings.Join(list, ",")
}

func (c *SupportBundleController) getTaintTolerationString(setting *longhorn.Setting) string {
	if setting == nil {
		return ""
	}
	return strings.ReplaceAll(setting.Value, ";", ",")
}

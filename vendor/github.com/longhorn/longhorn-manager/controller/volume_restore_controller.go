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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/longhorn/longhorn-manager/constant"
	"github.com/longhorn/longhorn-manager/datastore"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type VolumeRestoreController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds         *datastore.DataStore
	cacheSyncs []cache.InformerSynced
}

func NewVolumeRestoreController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
) *VolumeRestoreController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)

	vrsc := &VolumeRestoreController{
		baseController: newBaseController("longhorn-volume-restore", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-volume-restore-controller"}),
	}

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    vrsc.enqueueVolume,
		UpdateFunc: func(old, cur interface{}) { vrsc.enqueueVolume(cur) },
		DeleteFunc: vrsc.enqueueVolume,
	}, 0)
	vrsc.cacheSyncs = append(vrsc.cacheSyncs, ds.VolumeInformer.HasSynced)

	return vrsc
}

func (vrsc *VolumeRestoreController) enqueueVolume(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	vrsc.queue.Add(key)
}

func (vrsc *VolumeRestoreController) enqueueVolumeAfter(obj interface{}, duration time.Duration) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("enqueueVolumeAfter: failed to get key for object %#v: %v", obj, err))
		return
	}

	vrsc.queue.AddAfter(key, duration)
}

func (vrsc *VolumeRestoreController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer vrsc.queue.ShutDown()

	vrsc.logger.Info("Starting Longhorn restore controller")
	defer vrsc.logger.Info("Shut down Longhorn restore controller")

	if !cache.WaitForNamedCacheSync(vrsc.name, stopCh, vrsc.cacheSyncs...) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(vrsc.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (vrsc *VolumeRestoreController) worker() {
	for vrsc.processNextWorkItem() {
	}
}

func (vrsc *VolumeRestoreController) processNextWorkItem() bool {
	key, quit := vrsc.queue.Get()
	if quit {
		return false
	}
	defer vrsc.queue.Done(key)
	err := vrsc.syncHandler(key.(string))
	vrsc.handleErr(err, key)
	return true
}

func (vrsc *VolumeRestoreController) handleErr(err error, key interface{}) {
	if err == nil {
		vrsc.queue.Forget(key)
		return
	}

	log := vrsc.logger.WithField("Volume", key)
	handleReconcileErrorLogging(log, err, "Failed to sync Longhorn volume")
	vrsc.queue.AddRateLimited(key)
}

func (vrsc *VolumeRestoreController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync volume %v", vrsc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != vrsc.namespace {
		return nil
	}
	return vrsc.reconcile(name)
}

func (vrsc *VolumeRestoreController) reconcile(volName string) (err error) {
	vol, err := vrsc.ds.GetVolume(volName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if !vrsc.isResponsibleFor(vol) {
		return nil
	}

	va, err := vrsc.ds.GetLHVolumeAttachmentByVolumeName(volName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		vrsc.enqueueVolumeAfter(vol, constant.LonghornVolumeAttachmentNotFoundRetryPeriod)
		return nil
	}
	existingVA := va.DeepCopy()
	defer func() {
		if err != nil {
			return
		}
		if reflect.DeepEqual(existingVA.Spec, va.Spec) {
			return
		}

		if _, err = vrsc.ds.UpdateLHVolumeAttachment(va); err != nil {
			return
		}
	}()

	restoringAttachmentTicketID := longhorn.GetAttachmentTicketID(longhorn.AttacherTypeVolumeRestoreController, volName)

	if vol.Status.RestoreRequired {
		createOrUpdateAttachmentTicket(va, restoringAttachmentTicketID, vol.Status.OwnerID, longhorn.TrueValue, longhorn.AttacherTypeVolumeRestoreController)
	} else {
		delete(va.Spec.AttachmentTickets, restoringAttachmentTicketID)
	}

	return nil
}

func (vrsc *VolumeRestoreController) isResponsibleFor(vol *longhorn.Volume) bool {
	return vrsc.controllerID == vol.Status.OwnerID
}

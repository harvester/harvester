package controller

import (
	"context"
	"fmt"
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
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type KubernetesSecretController struct {
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

func NewKubernetesSecretController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string) *KubernetesSecretController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	ks := &KubernetesSecretController{
		baseController: newBaseController("longhorn-kubernetes-secret-controller", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "longhorn-kubernetes-secret-controller"}),
	}

	ds.SecretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ks.enqueueSecretChange,
		UpdateFunc: func(old, cur interface{}) {
			oldSecret := old.(*v1.Secret)
			curSecret := cur.(*v1.Secret)
			if curSecret.ResourceVersion == oldSecret.ResourceVersion {
				// Periodic resync will send update events for all known secrets.
				// Two different versions of the same secret will always have different RVs.
				// Ref to https://github.com/kubernetes/kubernetes/blob/c8ebc8ab75a9c36453cf6fa30990fd0a277d856d/pkg/controller/deployment/deployment_controller.go#L256-L263
				return
			}
			ks.enqueueSecretChange(cur)
		},
		DeleteFunc: ks.enqueueSecretChange,
	})
	ks.cacheSyncs = append(ks.cacheSyncs, ds.SecretInformer.HasSynced)

	return ks
}

func (ks *KubernetesSecretController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer ks.queue.ShutDown()

	ks.logger.Info("Start Longhorn Kubernetes secret controller")
	defer ks.logger.Info("Shutting down Longhorn Kubernetes secret controller")

	if !cache.WaitForNamedCacheSync(ks.name, stopCh, ks.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(ks.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (ks *KubernetesSecretController) worker() {
	for ks.processNextWorkItem() {
	}
}

func (ks *KubernetesSecretController) processNextWorkItem() bool {
	key, quit := ks.queue.Get()
	if quit {
		return false
	}
	defer ks.queue.Done(key)
	err := ks.syncHandler(key.(string))
	ks.handleErr(err, key)
	return true
}

func (ks *KubernetesSecretController) handleErr(err error, key interface{}) {
	if err == nil {
		ks.queue.Forget(key)
		return
	}

	if ks.queue.NumRequeues(key) < maxRetries {
		ks.logger.WithError(err).Warnf("Error syncing Secret %v", key)
		ks.queue.AddRateLimited(key)
		return
	}

	ks.logger.WithError(err).Warnf("Dropping Secret %v out of the queue", key)
	ks.queue.Forget(key)
	utilruntime.HandleError(err)
}

func (ks *KubernetesSecretController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync %v", ks.name, key)
	}()

	namespace, secretName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != ks.namespace {
		// Not ours, skip it
		return nil
	}

	if err := ks.reconcileSecret(namespace, secretName); err != nil {
		return err
	}
	return nil
}

func (ks *KubernetesSecretController) reconcileSecret(namespace, secretName string) error {
	// Get default backup target
	backupTarget, err := ks.ds.GetBackupTargetRO(types.DefaultBackupTargetName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		ks.logger.Warnf("Cannot found the %s backup target", types.DefaultBackupTargetName)
		return nil
	}

	backupType, err := util.CheckBackupType(backupTarget.Spec.BackupTargetURL)
	if err != nil || backupType != types.BackupStoreTypeS3 || backupTarget.Spec.CredentialSecret != secretName {
		// We only focus on backup target S3 and the credential secret setting matches to the current secret name
		return nil
	}

	secret, err := ks.ds.GetSecretRO(namespace, secretName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	awsIAMRoleArn := ""
	if secret != nil {
		awsIAMRoleArn = string(secret.Data[types.AWSIAMRoleArn])
	}

	// Annotates AWS IAM role arn to the manager as well as the replica instance managers
	if err := ks.annotateAWSIAMRoleArn(awsIAMRoleArn); err != nil {
		return err
	}
	// Trigger backup_target_controller once the credential secret changes
	return ks.triggerSyncBackupTarget(backupTarget)
}

func (ks *KubernetesSecretController) enqueueSecretChange(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	ks.queue.Add(key)
}

// annotateAWSIAMRoleArn ensures that the running pods of the manager as well as the replica instance managers.
// have the correct AWS IAM role arn assigned to them based on the passed `awsIAMRoleArn`
func (ks *KubernetesSecretController) annotateAWSIAMRoleArn(awsIAMRoleArn string) error {
	managerPods, err := ks.ds.ListManagerPods()
	if err != nil {
		return err
	}

	imPods, err := ks.ds.ListInstanceManagerPodsBy(ks.controllerID, "", longhorn.InstanceManagerTypeReplica)
	if err != nil {
		return err
	}

	pods := append(managerPods, imPods...)
	for _, pod := range pods {
		if pod.Spec.NodeName != ks.controllerID {
			continue
		}

		val, exist := pod.Annotations[types.AWSIAMRoleAnnotation]
		updateAnnotation := awsIAMRoleArn != "" && awsIAMRoleArn != val
		deleteAnnotation := awsIAMRoleArn == "" && exist
		if updateAnnotation {
			if pod.Annotations == nil {
				pod.Annotations = make(map[string]string)
			}
			pod.Annotations[types.AWSIAMRoleAnnotation] = awsIAMRoleArn
		} else if deleteAnnotation {
			delete(pod.Annotations, types.AWSIAMRoleAnnotation)
		} else {
			continue
		}

		if _, err = ks.kubeClient.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{}); err != nil {
			return err
		}

		ks.logger.Infof("AWS IAM role for pod %v/%v updated", pod.Namespace, pod.Name)
	}

	return nil
}

// triggerSyncBackupTarget sets the `spec.syncRequestedAt` trigger backup_target_controller
// to run reconcile loop
func (ks *KubernetesSecretController) triggerSyncBackupTarget(backupTarget *longhorn.BackupTarget) error {
	if backupTarget.Status.OwnerID != ks.controllerID {
		return nil
	}

	backupTarget.Spec.SyncRequestedAt = metav1.Time{Time: time.Now().UTC()}
	if _, err := ks.ds.UpdateBackupTarget(backupTarget); err != nil && !apierrors.IsConflict(errors.Cause(err)) {
		ks.logger.WithError(err).Warn("Failed to updating backup target")
	}
	ks.logger.Debug("Trigger sync backup target because the credential secret change")
	return nil
}

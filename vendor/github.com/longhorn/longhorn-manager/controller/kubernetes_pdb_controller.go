package controller

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
)

var targetingDeployment map[string]bool = map[string]bool{
	types.CSIAttacherName:                         true,
	types.CSIProvisionerName:                      true,
	types.LonghornAdmissionWebhookDeploymentName:  true,
	types.LonghornConversionWebhookDeploymentName: true,
}

type KubernetesPDBController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient clientset.Interface

	ds *datastore.DataStore

	cacheSyncs []cache.InformerSynced
}

func NewKubernetesPDBController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string) *KubernetesPDBController {

	pc := &KubernetesPDBController{
		baseController: newBaseController("kubernetes-pdb", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds:         ds,
		kubeClient: kubeClient,
	}

	ds.DeploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    pc.enqueueDeployment,
		UpdateFunc: func(old, cur interface{}) { pc.enqueueDeployment(cur) },
		DeleteFunc: pc.enqueueDeployment,
	})
	pc.cacheSyncs = append(pc.cacheSyncs, ds.DeploymentInformer.HasSynced)

	ds.VolumeInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    pc.enqueueVolume,
		UpdateFunc: func(old, cur interface{}) { pc.enqueueVolume(cur) },
		DeleteFunc: pc.enqueueVolume,
	}, 0)
	pc.cacheSyncs = append(pc.cacheSyncs, ds.VolumeInformer.HasSynced)

	return pc
}

func (pc *KubernetesPDBController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer pc.queue.ShutDown()

	pc.logger.Infof("Starting Kubernetes PDB controller")
	defer pc.logger.Infof("Shut down Kubernetes PDB controller")

	if !cache.WaitForNamedCacheSync(pc.name, stopCh, pc.cacheSyncs...) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(pc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (pc *KubernetesPDBController) worker() {
	for pc.processNextWorkItem() {
	}
}

func (pc *KubernetesPDBController) processNextWorkItem() bool {
	key, quit := pc.queue.Get()
	if quit {
		return false
	}
	defer pc.queue.Done(key)
	err := pc.syncHandler(key.(string))
	pc.handleErr(err, key)
	return true
}

func (pc *KubernetesPDBController) handleErr(err error, key interface{}) {
	if err == nil {
		pc.queue.Forget(key)
		return
	}

	pc.logger.WithError(err).Warnf("Error syncing PDB for %v", key)
	pc.queue.AddRateLimited(key)
}

func (pc *KubernetesPDBController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: failed to sync PDB for %v", pc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != pc.namespace {
		return nil
	}

	if _, ok := targetingDeployment[name]; !ok {
		return nil
	}
	return pc.reconcile(name)
}

func (pc *KubernetesPDBController) reconcile(name string) (err error) {
	pdbName := name
	pdb, err := pc.ds.GetPDBRO(pdbName)
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}

	deployment, err := pc.ds.GetDeployment(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return deletePDBObject(pc.logger, pc.ds, pdb)
		}
		return err
	}

	volumes, err := pc.ds.ListVolumes()
	if err != nil {
		return err
	}
	hasInUsedVolume := false
	for _, vol := range volumes {
		if vol.Status.State != longhorn.VolumeStateDetached {
			hasInUsedVolume = true
			break
		}
	}
	if !hasInUsedVolume {
		return deletePDBObject(pc.logger, pc.ds, pdb)
	}

	if pdb != nil {
		return nil
	}
	pdb = generatePDBManifest(pdbName, pc.namespace, deployment.Spec.Selector)
	if _, err := pc.ds.CreatePDB(pdb); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	pc.logger.Infof("Created %v PDB", pdbName)

	return nil
}

func (pc *KubernetesPDBController) enqueueDeployment(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}
	pc.queue.Add(key)
}

func (pc *KubernetesPDBController) enqueueVolume(obj interface{}) {
	for deployment := range targetingDeployment {
		pc.queue.Add(pc.namespace + "/" + deployment)
	}
}

func generatePDBManifest(name, namespace string, selector *metav1.LabelSelector) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector:     selector,
			MinAvailable: &intstr.IntOrString{IntVal: 1},
		},
	}
}

func deletePDBObject(log logrus.FieldLogger, ds *datastore.DataStore, pdb *policyv1.PodDisruptionBudget) error {
	if pdb == nil {
		return nil
	}
	err := ds.DeletePDB(pdb.Name)
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}
	log.Infof("Deleted %v PDB", pdb.Name)
	return nil
}

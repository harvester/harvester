package controller

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions"
)

var (
	Workers              = 5
	longhornFinalizerKey = longhorn.SchemeGroupVersion.Group
)

func StartControllers(logger logrus.FieldLogger, stopCh chan struct{}, controllerID, serviceAccount, managerImage, kubeconfigPath, version string, proxyConnCounter util.Counter) (*datastore.DataStore, *WebsocketController, error) {
	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		logrus.Warnf("Cannot detect pod namespace, environment variable %v is missing, "+
			"using default namespace", types.EnvPodNamespace)
		namespace = corev1.NamespaceDefault
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to get client config")
	}

	config.Burst = 100
	config.QPS = 50

	kubeClient, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to get k8s client")
	}

	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to get clientset")
	}

	scheme := runtime.NewScheme()
	if err := longhorn.SchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, nil, errors.Wrap(err, "unable to create scheme")
	}

	// TODO: there shouldn't be a need for a 30s resync period unless our code is buggy and our controllers aren't really
	//  level based. What we are effectively doing with this is hiding faulty logic in production.
	//  Another reason for increasing this substantially, is that it introduces a lot of unnecessary work and will
	//  lead to scalability problems, since we dump the whole cache of each object back in to the reconciler every 30 seconds.
	//  if a specific controller requires a periodic resync, one enable it only for that informer, add a resync to the event handler, go routine, etc.
	//  some refs to look at: https://github.com/kubernetes-sigs/controller-runtime/issues/521
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	lhInformerFactory := lhinformers.NewSharedInformerFactory(lhClient, time.Second*30)

	ds := datastore.NewDataStore(lhInformerFactory, lhClient, kubeInformerFactory, kubeClient, namespace)

	rc := NewReplicaController(logger, ds, scheme, kubeClient, namespace, controllerID)
	ec := NewEngineController(logger, ds, scheme, kubeClient, &engineapi.EngineCollection{}, namespace, controllerID, proxyConnCounter)
	vc := NewVolumeController(logger, ds, scheme, kubeClient, namespace, controllerID, proxyConnCounter)
	ic := NewEngineImageController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	nc := NewNodeController(logger, ds, scheme, kubeClient, namespace, controllerID)
	ws := NewWebsocketController(logger, ds)
	sc := NewSettingController(logger, ds, scheme, kubeClient, namespace, controllerID, version)
	btc := NewBackupTargetController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	bvc := NewBackupVolumeController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	bc := NewBackupController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	imc := NewInstanceManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	smc := NewShareManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	bic := NewBackingImageController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	bimc := NewBackingImageManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	bidsc := NewBackingImageDataSourceController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, proxyConnCounter)
	rjc := NewRecurringJobController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, managerImage)
	oc := NewOrphanController(logger, ds, scheme, kubeClient, controllerID, namespace)
	snapc := NewSnapshotController(logger, ds, scheme, kubeClient, namespace, controllerID, &engineapi.EngineCollection{}, proxyConnCounter)
	kpvc := NewKubernetesPVController(logger, ds, scheme, kubeClient, controllerID)
	knc := NewKubernetesNodeController(logger, ds, scheme, kubeClient, controllerID)
	kpc := NewKubernetesPodController(logger, ds, scheme, kubeClient, controllerID)
	kcfmc := NewKubernetesConfigMapController(logger, ds, scheme, kubeClient, controllerID, namespace)
	ksc := NewKubernetesSecretController(logger, ds, scheme, kubeClient, controllerID, namespace)

	go kubeInformerFactory.Start(stopCh)
	go lhInformerFactory.Start(stopCh)
	if !ds.Sync(stopCh) {
		return nil, nil, fmt.Errorf("datastore cache sync up failed")
	}
	go rc.Run(Workers, stopCh)
	go ec.Run(Workers, stopCh)
	go vc.Run(Workers, stopCh)
	go ic.Run(Workers, stopCh)
	go nc.Run(Workers, stopCh)
	go ws.Run(stopCh)
	go sc.Run(stopCh)
	go imc.Run(Workers, stopCh)
	go smc.Run(Workers, stopCh)
	go bic.Run(Workers, stopCh)
	go bimc.Run(Workers, stopCh)
	go bidsc.Run(Workers, stopCh)
	go btc.Run(Workers, stopCh)
	go bvc.Run(Workers, stopCh)
	go bc.Run(Workers, stopCh)
	go rjc.Run(Workers, stopCh)
	go oc.Run(Workers, stopCh)
	go snapc.Run(Workers, stopCh)

	go kpvc.Run(Workers, stopCh)
	go knc.Run(Workers, stopCh)
	go kpc.Run(Workers, stopCh)
	go kcfmc.Run(Workers, stopCh)
	go ksc.Run(Workers, stopCh)

	return ds, ws, nil
}

func ParseResourceRequirement(val string) (*corev1.ResourceRequirements, error) {
	quantity, err := resource.ParseQuantity(val)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse value %v to a quantity", val)
	}
	if quantity.IsZero() {
		return nil, nil
	}
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: quantity,
		},
	}, nil
}

func GetInstanceManagerCPURequirement(ds *datastore.DataStore, imName string) (*corev1.ResourceRequirements, error) {
	im, err := ds.GetInstanceManager(imName)
	if err != nil {
		return nil, err
	}

	lhNode, err := ds.GetNode(im.Spec.NodeID)
	if err != nil {
		return nil, err
	}
	kubeNode, err := ds.GetKubernetesNode(im.Spec.NodeID)
	if err != nil {
		return nil, err
	}

	allocatableMilliCPU := float64(kubeNode.Status.Allocatable.Cpu().MilliValue())
	switch im.Spec.Type {
	case longhorn.InstanceManagerTypeEngine:
		emCPURequest := lhNode.Spec.EngineManagerCPURequest
		if emCPURequest == 0 {
			emCPUSetting, err := ds.GetSetting(types.SettingNameGuaranteedEngineManagerCPU)
			if err != nil {
				return nil, err
			}
			emCPUPercentage, err := strconv.ParseFloat(emCPUSetting.Value, 64)
			if err != nil {
				return nil, err
			}
			emCPURequest = int(math.Round(allocatableMilliCPU * emCPUPercentage / 100.0))
		}
		return ParseResourceRequirement(fmt.Sprintf("%dm", emCPURequest))
	case longhorn.InstanceManagerTypeReplica:
		rmCPURequest := lhNode.Spec.ReplicaManagerCPURequest
		if rmCPURequest == 0 {
			rmCPUSetting, err := ds.GetSetting(types.SettingNameGuaranteedReplicaManagerCPU)
			if err != nil {
				return nil, err
			}
			rmCPUPercentage, err := strconv.ParseFloat(rmCPUSetting.Value, 64)
			if err != nil {
				return nil, err
			}
			rmCPURequest = int(math.Round(allocatableMilliCPU * rmCPUPercentage / 100.0))
		}
		return ParseResourceRequirement(fmt.Sprintf("%dm", rmCPURequest))
	default:
		return nil, fmt.Errorf("instance manager %v has unknown type %v", im.Name, im.Spec.Type)
	}
}

func isControllerResponsibleFor(controllerID string, ds *datastore.DataStore, name, preferredOwnerID, currentOwnerID string) bool {
	// we use this approach so that if there is an issue with the data store
	// we don't accidentally transfer ownership
	isOwnerUnavailable := func(node string) bool {
		isUnavailable, err := ds.IsNodeDownOrDeletedOrMissingManager(node)
		if node != "" && err != nil {
			logrus.Errorf("Error while checking IsNodeDownOrDeletedOrMissingManager for object %v, node %v: %v", name, node, err)
		}
		return node == "" || isUnavailable
	}

	isPreferredOwner := controllerID == preferredOwnerID
	continueToBeOwner := currentOwnerID == controllerID && isOwnerUnavailable(preferredOwnerID)
	requiresNewOwner := isOwnerUnavailable(currentOwnerID) && isOwnerUnavailable(preferredOwnerID)
	return isPreferredOwner || continueToBeOwner || requiresNewOwner
}

// EnhancedDefaultControllerRateLimiter is an enhanced version of workqueue.DefaultControllerRateLimiter()
// See https://github.com/longhorn/longhorn/issues/1058 for details
func EnhancedDefaultControllerRateLimiter() workqueue.RateLimiter {
	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		// 100 qps, 1000 bucket size
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(100), 1000)},
	)
}

func IsSameGuaranteedCPURequirement(a, b *corev1.ResourceRequirements) bool {
	var aQ, bQ resource.Quantity
	if a != nil && a.Requests != nil {
		aQ = a.Requests[corev1.ResourceCPU]
	}
	if b != nil && b.Requests != nil {
		bQ = b.Requests[corev1.ResourceCPU]
	}
	return (&aQ).Cmp(bQ) == 0
}

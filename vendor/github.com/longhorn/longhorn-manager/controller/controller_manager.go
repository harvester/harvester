package controller

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/workqueue"

	corev1 "k8s.io/api/core/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/engineapi"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
	"github.com/longhorn/longhorn-manager/util/client"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

var (
	Workers              = 5
	longhornFinalizerKey = longhorn.SchemeGroupVersion.Group
)

// StartControllers initiates all Longhorn component controllers and monitors to manage the creating, updating, and deletion of Longhorn resources
func StartControllers(logger logrus.FieldLogger, clients *client.Clients,
	controllerID, serviceAccount, managerImage, backingImageManagerImage, shareManagerImage,
	kubeconfigPath, version string, proxyConnCounter util.Counter) (*WebsocketController, error) {
	namespace := clients.Namespace
	kubeClient := clients.Clients.K8s
	metricsClient := clients.MetricsClient
	ds := clients.Datastore
	scheme := clients.Scheme
	stopCh := clients.StopCh

	// Longhorn controllers
	replicaController := NewReplicaController(logger, ds, scheme, kubeClient, namespace, controllerID)
	engineController := NewEngineController(logger, ds, scheme, kubeClient, &engineapi.EngineCollection{}, namespace, controllerID, proxyConnCounter)
	volumeController := NewVolumeController(logger, ds, scheme, kubeClient, namespace, controllerID, shareManagerImage, proxyConnCounter)
	engineImageController := NewEngineImageController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	nodeController := NewNodeController(logger, ds, scheme, kubeClient, namespace, controllerID)
	websocketController := NewWebsocketController(logger, ds)
	settingController := NewSettingController(logger, ds, scheme, kubeClient, metricsClient, namespace, controllerID, version)
	backupTargetController := NewBackupTargetController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	backupVolumeController := NewBackupVolumeController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	backupController := NewBackupController(logger, ds, scheme, kubeClient, controllerID, namespace, proxyConnCounter)
	instanceManagerController := NewInstanceManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	shareManagerController := NewShareManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount)
	backingImageController := NewBackingImageController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, backingImageManagerImage)
	backingImageManagerController := NewBackingImageManagerController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, backingImageManagerImage)
	backingImageDataSourceController := NewBackingImageDataSourceController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, backingImageManagerImage, proxyConnCounter)
	recurringJobController := NewRecurringJobController(logger, ds, scheme, kubeClient, namespace, controllerID, serviceAccount, managerImage)
	orphanController := NewOrphanController(logger, ds, scheme, kubeClient, controllerID, namespace)
	snapshotController := NewSnapshotController(logger, ds, scheme, kubeClient, namespace, controllerID, &engineapi.EngineCollection{}, proxyConnCounter)
	supportBundleController := NewSupportBundleController(logger, ds, scheme, kubeClient, controllerID, namespace, serviceAccount)
	systemBackupController := NewSystemBackupController(logger, ds, scheme, kubeClient, namespace, controllerID, managerImage)
	systemRestoreController := NewSystemRestoreController(logger, ds, scheme, kubeClient, namespace, controllerID)
	volumeAttachmentController := NewLonghornVolumeAttachmentController(logger, ds, scheme, kubeClient, controllerID, namespace)
	volumeRestoreController := NewVolumeRestoreController(logger, ds, scheme, kubeClient, controllerID, namespace)
	volumeRebuildingController := NewVolumeRebuildingController(logger, ds, scheme, kubeClient, controllerID, namespace)
	volumeEvictionController := NewVolumeEvictionController(logger, ds, scheme, kubeClient, controllerID, namespace)
	volumeCloneController := NewVolumeCloneController(logger, ds, scheme, kubeClient, controllerID, namespace)
	volumeExpansionController := NewVolumeExpansionController(logger, ds, scheme, kubeClient, controllerID, namespace)

	// Kubernetes controllers
	kubernetesPVController := NewKubernetesPVController(logger, ds, scheme, kubeClient, controllerID)
	kubernetesNodeController := NewKubernetesNodeController(logger, ds, scheme, kubeClient, controllerID)
	kubernetesPodController := NewKubernetesPodController(logger, ds, scheme, kubeClient, controllerID)
	kubernetesConfigMapController := NewKubernetesConfigMapController(logger, ds, scheme, kubeClient, controllerID, namespace)
	kubernetesSecretController := NewKubernetesSecretController(logger, ds, scheme, kubeClient, controllerID, namespace)
	kubernetesPDBController := NewKubernetesPDBController(logger, ds, kubeClient, controllerID, namespace)

	// Start goroutines for Longhorn controllers
	go replicaController.Run(Workers, stopCh)
	go engineController.Run(Workers, stopCh)
	go volumeController.Run(Workers, stopCh)
	go engineImageController.Run(Workers, stopCh)
	go nodeController.Run(Workers, stopCh)
	go websocketController.Run(stopCh)
	go settingController.Run(stopCh)
	go instanceManagerController.Run(Workers, stopCh)
	go shareManagerController.Run(Workers, stopCh)
	go backingImageController.Run(Workers, stopCh)
	go backingImageManagerController.Run(Workers, stopCh)
	go backingImageDataSourceController.Run(Workers, stopCh)
	go backupTargetController.Run(Workers, stopCh)
	go backupVolumeController.Run(Workers, stopCh)
	go backupController.Run(Workers, stopCh)
	go recurringJobController.Run(Workers, stopCh)
	go orphanController.Run(Workers, stopCh)
	go snapshotController.Run(Workers, stopCh)
	go supportBundleController.Run(Workers, stopCh)
	go systemBackupController.Run(Workers, stopCh)
	go systemRestoreController.Run(Workers, stopCh)
	go volumeAttachmentController.Run(Workers, stopCh)
	go volumeRestoreController.Run(Workers, stopCh)
	go volumeRebuildingController.Run(Workers, stopCh)
	go volumeEvictionController.Run(Workers, stopCh)
	go volumeCloneController.Run(Workers, stopCh)
	go volumeExpansionController.Run(Workers, stopCh)

	// Start goroutines for Kubernetes controllers
	go kubernetesPVController.Run(Workers, stopCh)
	go kubernetesNodeController.Run(Workers, stopCh)
	go kubernetesPodController.Run(Workers, stopCh)
	go kubernetesConfigMapController.Run(Workers, stopCh)
	go kubernetesSecretController.Run(Workers, stopCh)
	go kubernetesPDBController.Run(Workers, stopCh)

	return websocketController, nil
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

	cpuRequest := lhNode.Spec.InstanceManagerCPURequest
	guaranteedCPUSettingName := types.SettingNameGuaranteedInstanceManagerCPU
	if cpuRequest == 0 {
		guaranteedCPUSetting, err := ds.GetSetting(guaranteedCPUSettingName)
		if err != nil {
			return nil, err
		}
		guaranteedCPUPercentage, err := strconv.ParseFloat(guaranteedCPUSetting.Value, 64)
		if err != nil {
			return nil, err
		}
		allocatableMilliCPU := float64(kubeNode.Status.Allocatable.Cpu().MilliValue())
		cpuRequest = int(math.Round(allocatableMilliCPU * guaranteedCPUPercentage / 100.0))
	}
	return ParseResourceRequirement(fmt.Sprintf("%dm", cpuRequest))
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

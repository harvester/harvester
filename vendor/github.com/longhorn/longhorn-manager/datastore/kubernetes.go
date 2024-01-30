package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	// KubeStatusPollCount is the number of retry to validate The KubernetesStatus
	KubeStatusPollCount = 5
	// KubeStatusPollInterval is the waiting time between each KubeStatusPollCount
	KubeStatusPollInterval = 1 * time.Second

	PodProbeInitialDelay             = 3
	PodProbeTimeoutSeconds           = PodProbePeriodSeconds - 1
	PodProbePeriodSeconds            = 5
	PodLivenessProbeFailureThreshold = 3
)

func labelMapToLabelSelector(labels map[string]string) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: labels,
	})
}

func (s *DataStore) GetManagerLabel() map[string]string {
	return types.GetManagerLabels()
}

func (s *DataStore) getManagerSelector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: s.GetManagerLabel(),
	})
}

func (s *DataStore) GetSupportBundleManagerLabel(supportBundle *longhorn.SupportBundle) map[string]string {
	return s.getSupportBundleManagerLabel(supportBundle)
}

func (s *DataStore) getSupportBundleManagerLabel(supportBundle *longhorn.SupportBundle) map[string]string {
	return map[string]string{
		"app":                              types.SupportBundleManagerApp,
		types.SupportBundleManagerLabelKey: supportBundle.Name,
	}
}

func (s *DataStore) getSupportBundleManagerSelector(supportBundle *longhorn.SupportBundle) (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: s.getSupportBundleManagerLabel(supportBundle),
	})
}

// GetManagerNodeIPMap returns an object contains podIPs from list
// of running pods with app=longhorn-manager
func (s *DataStore) GetManagerNodeIPMap() (map[string]string, error) {
	selector, err := s.getManagerSelector()
	if err != nil {
		return nil, err
	}
	podList, err := s.podLister.Pods(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}
	if len(podList) == 0 {
		return nil, fmt.Errorf("cannot find manager pods by label %v", s.GetManagerLabel())
	}
	nodeIPMap := make(map[string]string)
	for _, pod := range podList {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nodeIPMap[pod.Spec.NodeName] != "" {
			return nil, fmt.Errorf("multiple managers on the node %v", pod.Spec.NodeName)
		}
		nodeIPMap[pod.Spec.NodeName] = pod.Status.PodIP
	}
	return nodeIPMap, nil
}

// GetCronJobROByRecurringJob returns read-only CronJob for the recurring job
func (s *DataStore) GetCronJobROByRecurringJob(recurringJob *longhorn.RecurringJob) (*batchv1.CronJob, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetCronJobLabels(
			&longhorn.RecurringJobSpec{
				Name: recurringJob.Name,
				Task: longhorn.RecurringJobType(recurringJob.Spec.Task),
			},
		),
	})
	if err != nil {
		return nil, err
	}
	itemMap := map[string]*batchv1.CronJob{
		recurringJob.Name: nil,
	}
	list, err := s.cronJobLister.CronJobs(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}
	for _, cronJob := range list {
		if itemMap[recurringJob.Name] != nil {
			return nil, fmt.Errorf("multiple cronjob found for %v recurring job", recurringJob.Name)
		}
		itemMap[recurringJob.Name] = cronJob
	}
	return itemMap[recurringJob.Name], nil
}

// CreateCronJob creates a CronJob resource
func (s *DataStore) CreateCronJob(cronJob *batchv1.CronJob) (*batchv1.CronJob, error) {
	return s.kubeClient.BatchV1().CronJobs(s.namespace).Create(context.TODO(), cronJob, metav1.CreateOptions{})
}

// UpdateCronJob updates CronJob resource
func (s *DataStore) UpdateCronJob(cronJob *batchv1.CronJob) (*batchv1.CronJob, error) {
	return s.kubeClient.BatchV1().CronJobs(s.namespace).Update(context.TODO(), cronJob, metav1.UpdateOptions{})
}

// DeleteCronJob delete CronJob for the given name and namespace.
// The dependents will be deleted in the background
func (s *DataStore) DeleteCronJob(cronJobName string) error {
	propagation := metav1.DeletePropagationBackground
	err := s.kubeClient.BatchV1().CronJobs(s.namespace).Delete(context.TODO(), cronJobName,
		metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// CreateEngineImageDaemonSet sets EngineImage labels in DaemonSet label and
// creates a DaemonSet resource in the given namespace
func (s *DataStore) CreateEngineImageDaemonSet(ds *appsv1.DaemonSet) error {
	if ds.Labels == nil {
		ds.Labels = map[string]string{}
	}
	for k, v := range types.GetEngineImageLabels(types.GetEngineImageNameFromDaemonSetName(ds.Name)) {
		ds.Labels[k] = v
	}
	if _, err := s.kubeClient.AppsV1().DaemonSets(s.namespace).Create(context.TODO(), ds, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

// GetEngineImageDaemonSet get DaemonSet for the given name and namespace, and
// returns a new DaemonSet object
func (s *DataStore) GetEngineImageDaemonSet(name string) (*appsv1.DaemonSet, error) {
	resultRO, err := s.daemonSetLister.DaemonSets(s.namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) ListEngineImageDaemonSetPodsFromEngineImageName(EIName string) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetEIDaemonSetLabelSelector(EIName),
	})
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

// CreatePDB creates a PodDisruptionBudget resource for the given PDB object and namespace
func (s *DataStore) CreatePDB(pdp *policyv1.PodDisruptionBudget) (*policyv1.PodDisruptionBudget, error) {
	return s.kubeClient.PolicyV1().PodDisruptionBudgets(s.namespace).Create(context.TODO(), pdp, metav1.CreateOptions{})
}

// DeletePDB deletes PodDisruptionBudget for the given name and namespace
func (s *DataStore) DeletePDB(name string) error {
	return s.kubeClient.PolicyV1().PodDisruptionBudgets(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// GetPDBRO gets PDB for the given name and namespace.
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetPDBRO(name string) (*policyv1.PodDisruptionBudget, error) {
	return s.podDisruptionBudgetLister.PodDisruptionBudgets(s.namespace).Get(name)
}

// ListPDBs gets a map of PDB in s.namespace
func (s *DataStore) ListPDBs() (map[string]*policyv1.PodDisruptionBudget, error) {
	itemMap := map[string]*policyv1.PodDisruptionBudget{}

	list, err := s.podDisruptionBudgetLister.PodDisruptionBudgets(s.namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, itemRO := range list {
		// Cannot use cached object from lister
		itemMap[itemRO.Name] = itemRO.DeepCopy()
	}
	return itemMap, nil
}

// CreatePod creates a Pod resource for the given pod object and namespace
func (s *DataStore) CreatePod(pod *corev1.Pod) (*corev1.Pod, error) {
	return s.kubeClient.CoreV1().Pods(s.namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
}

// DeletePod deletes Pod for the given name and namespace
func (s *DataStore) DeletePod(name string) error {
	return s.kubeClient.CoreV1().Pods(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// UpdatePod updates Pod for the given Pod object and namespace
func (s *DataStore) UpdatePod(obj *corev1.Pod) (*corev1.Pod, error) {
	return s.kubeClient.CoreV1().Pods(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
}

// DeleteLease deletes Lease with the given name in s.namespace
func (s *DataStore) DeleteLease(name string) error {
	return s.kubeClient.CoordinationV1().Leases(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// GetStorageClassRO gets StorageClass with the given name
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetStorageClassRO(scName string) (*storagev1.StorageClass, error) {
	return s.storageclassLister.Get(scName)
}

// GetStorageClass returns a new StorageClass object for the given name
func (s *DataStore) GetStorageClass(name string) (*storagev1.StorageClass, error) {
	resultRO, err := s.GetStorageClassRO(name)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListStorageClassesInPersistentVolumesWithLonghornProvisioner returns a list
// of StorageClasses used by PersistenVolumes with provisioner "driver.longhorn.io".
func (s *DataStore) ListStorageClassesInPersistentVolumesWithLonghornProvisioner() ([]string, error) {
	pvList, err := s.ListPersistentVolumesRO()
	if err != nil {
		return nil, err
	}

	scList := []string{}
	for _, pv := range pvList {
		if pv.Spec.CSI == nil {
			continue
		}
		if pv.Spec.CSI.Driver != types.LonghornDriverName {
			continue
		}

		scList = append(scList, pv.Spec.StorageClassName)
	}

	return scList, nil
}

// DeleteStorageClass deletes StorageClass with the given name
func (s *DataStore) DeleteStorageClass(scName string) error {
	return s.kubeClient.StorageV1().StorageClasses().Delete(context.TODO(), scName, metav1.DeleteOptions{})
}

// CreateStorageClass creates StorageClass with the given object
func (s *DataStore) CreateStorageClass(sc *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	return s.kubeClient.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
}

// UpdateStorageClass updates the StorageClass for the given StorageClass object
func (s *DataStore) UpdateStorageClass(obj *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	return s.kubeClient.StorageV1().StorageClasses().Update(context.TODO(), obj, metav1.UpdateOptions{})
}

// ListPodsRO returns a list of all Pods for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListPodsRO(namespace string) ([]*corev1.Pod, error) {
	return s.podLister.Pods(namespace).List(labels.Everything())
}

// GetPod returns a mutable Pod object for the given name and namespace
func (s *DataStore) GetPod(name string) (*corev1.Pod, error) {
	resultRO, err := s.podLister.Pods(s.namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return resultRO.DeepCopy(), nil
}

func (s *DataStore) GetPodRO(namespace, name string) (*corev1.Pod, error) {
	return s.podLister.Pods(namespace).Get(name)
}

// GetPodContainerLog dumps the log of a container in a Pod object for the given name and namespace.
// Be careful that this function will directly talk with the API server.
func (s *DataStore) GetPodContainerLog(podName, containerName string) ([]byte, error) {
	podLogOpts := &corev1.PodLogOptions{}
	if containerName != "" {
		podLogOpts.Container = containerName
	}
	return s.kubeClient.CoreV1().Pods(s.namespace).GetLogs(podName, podLogOpts).DoRaw(context.TODO())
}

// CreateDaemonSet creates a DaemonSet resource with the given DaemonSet object in the Longhorn namespace
func (s *DataStore) CreateDaemonSet(daemonSet *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	return s.kubeClient.AppsV1().DaemonSets(s.namespace).Create(context.TODO(), daemonSet, metav1.CreateOptions{})
}

// GetDaemonSet gets the DaemonSet for the given name and namespace
func (s *DataStore) GetDaemonSet(name string) (*appsv1.DaemonSet, error) {
	return s.daemonSetLister.DaemonSets(s.namespace).Get(name)
}

// ListDaemonSet gets a list of all DaemonSet for the given namespace
func (s *DataStore) ListDaemonSet() ([]*appsv1.DaemonSet, error) {
	return s.daemonSetLister.DaemonSets(s.namespace).List(labels.Everything())
}

func (s *DataStore) ListDaemonSetWithLabels(labels map[string]string) ([]*appsv1.DaemonSet, error) {
	selector, err := labelMapToLabelSelector(labels)
	if err != nil {
		return nil, err
	}
	return s.daemonSetLister.DaemonSets(s.namespace).List(selector)
}

// UpdateDaemonSet updates the DaemonSet for the given DaemonSet object and namespace
func (s *DataStore) UpdateDaemonSet(obj *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	return s.kubeClient.AppsV1().DaemonSets(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
}

// DeleteDaemonSet deletes DaemonSet for the given name and namespace.
// The dependents will be deleted in the foreground
func (s *DataStore) DeleteDaemonSet(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.kubeClient.AppsV1().DaemonSets(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// CreateDeployment creates Deployment with the given object
func (s *DataStore) CreateDeployment(obj *appsv1.Deployment) (*appsv1.Deployment, error) {
	return s.kubeClient.AppsV1().Deployments(s.namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
}

// GetDeployment gets the Deployment for the given name and namespace
func (s *DataStore) GetDeployment(name string) (*appsv1.Deployment, error) {
	return s.deploymentLister.Deployments(s.namespace).Get(name)
}

// ListDeployment gets a list of all Deployment for the given namespace
func (s *DataStore) ListDeployment() ([]*appsv1.Deployment, error) {
	return s.deploymentLister.Deployments(s.namespace).List(labels.Everything())
}

func (s *DataStore) ListDeploymentWithLabels(labels map[string]string) ([]*appsv1.Deployment, error) {
	selector, err := labelMapToLabelSelector(labels)
	if err != nil {
		return nil, err
	}
	return s.deploymentLister.Deployments(s.namespace).List(selector)
}

// UpdateDeployment updates Deployment for the given Deployment object and namespace
func (s *DataStore) UpdateDeployment(obj *appsv1.Deployment) (*appsv1.Deployment, error) {
	return s.kubeClient.AppsV1().Deployments(s.namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
}

// DeleteDeployment deletes Deployment for the given name and namespace.
// The dependents will be deleted in the foreground
func (s *DataStore) DeleteDeployment(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.kubeClient.AppsV1().Deployments(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// DeleteCSIDriver deletes CSIDriver for the given name and namespace
func (s *DataStore) DeleteCSIDriver(name string) error {
	return s.kubeClient.StorageV1().CSIDrivers().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// ListManagerPods returns a list of Pods marked with app=longhorn-manager
func (s *DataStore) ListManagerPods() ([]*corev1.Pod, error) {
	selector, err := s.getManagerSelector()
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

func getInstanceManagerComponentSelector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetInstanceManagerComponentLabel(),
	})
}

// ListInstanceManagerPods returns a list of Pod marked with component=instance-manager
func (s *DataStore) ListInstanceManagerPods() ([]*corev1.Pod, error) {
	selector, err := getInstanceManagerComponentSelector()
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

// ListInstanceManagerPodsBy returns a list of instance manager pods that fulfill the below conditions
func (s *DataStore) ListInstanceManagerPodsBy(node string, image string, imType longhorn.InstanceManagerType) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetInstanceManagerLabels(node, image, imType),
	})
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

func getShareManagerComponentSelector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetShareManagerComponentLabel(),
	})
}

func (s *DataStore) ListShareManagerPods() ([]*corev1.Pod, error) {
	selector, err := getShareManagerComponentSelector()
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

func (s *DataStore) ListBackingImageManagerPods() ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: types.GetBackingImageManagerLabels("", ""),
	})
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

// ListSupportBundleManagerPods returns a list of support bundle manager Pods
func (s *DataStore) ListSupportBundleManagerPods(supportBundle *longhorn.SupportBundle) ([]*corev1.Pod, error) {
	selector, err := s.getSupportBundleManagerSelector(supportBundle)
	if err != nil {
		return nil, err
	}
	return s.ListPodsBySelector(selector)
}

func (s *DataStore) GetSupportBundleManagerPod(supportBundle *longhorn.SupportBundle) (*corev1.Pod, error) {
	supportBundleManagerPods, err := s.ListSupportBundleManagerPods(supportBundle)
	if err != nil {
		return nil, err
	}

	if count := len(supportBundleManagerPods); count == 0 {
		return nil, fmt.Errorf("cannot find support bundle manager pod")
	} else if count > 1 {
		return nil, fmt.Errorf("found unexpected number of %v support bundle manager pod", count)
	}

	return supportBundleManagerPods[0], nil
}

func (s *DataStore) ListPodsBySelector(selector labels.Selector) ([]*corev1.Pod, error) {
	podList, err := s.podLister.Pods(s.namespace).List(selector)
	if err != nil {
		return nil, err
	}

	res := []*corev1.Pod{}
	for _, item := range podList {
		res = append(res, item.DeepCopy())
	}
	return res, nil
}

// ListKubeNodesRO returns a list of all Kubernetes Nodes for the given namespace,
// the list contains direct references to the internal cache objects and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListKubeNodesRO() ([]*corev1.Node, error) {
	return s.kubeNodeLister.List(labels.Everything())
}

// GetKubernetesNode gets the Node from the index for the given name
func (s *DataStore) GetKubernetesNode(name string) (*corev1.Node, error) {
	return s.kubeNodeLister.Get(name)
}

// IsKubeNodeUnschedulable checks if the Kubernetes Node resource is
// unschedulable
func (s *DataStore) IsKubeNodeUnschedulable(nodeName string) (bool, error) {
	kubeNode, err := s.GetKubernetesNode(nodeName)
	if err != nil {
		return false, err
	}
	return kubeNode.Spec.Unschedulable, nil
}

// CreatePersistentVolume creates a PersistentVolume resource for the given
// PersistentVolume object
func (s *DataStore) CreatePersistentVolume(pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	return s.kubeClient.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
}

// DeletePersistentVolume deletes the PersistentVolume for the given
// PersistentVolume name
func (s *DataStore) DeletePersistentVolume(pvName string) error {
	return s.kubeClient.CoreV1().PersistentVolumes().Delete(context.TODO(), pvName, metav1.DeleteOptions{})
}

// UpdatePersistentVolume updates the PersistentVolume for the given
// PersistentVolume object
func (s *DataStore) UpdatePersistentVolume(pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	return s.kubeClient.CoreV1().PersistentVolumes().Update(context.TODO(), pv, metav1.UpdateOptions{})
}

// GetPersistentVolumeRO gets the PersistentVolume from the index for the
// given name
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetPersistentVolumeRO(pvName string) (*corev1.PersistentVolume, error) {
	return s.persistentVolumeLister.Get(pvName)
}

// GetPersistentVolume gets a mutable PersistentVolume for the given name
func (s *DataStore) GetPersistentVolume(pvName string) (*corev1.PersistentVolume, error) {
	resultRO, err := s.persistentVolumeLister.Get(pvName)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListPersistentVolumesRO gets a list of PersistentVolumes.
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListPersistentVolumesRO() ([]*corev1.PersistentVolume, error) {
	return s.persistentVolumeLister.List(labels.Everything())
}

// CreatePersistentVolumeClaim creates a PersistentVolumeClaim resource
// for the given PersistentVolumeclaim object and namespace
func (s *DataStore) CreatePersistentVolumeClaim(ns string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return s.kubeClient.CoreV1().PersistentVolumeClaims(ns).Create(context.TODO(), pvc, metav1.CreateOptions{})
}

// DeletePersistentVolumeClaim deletes the PersistentVolumeClaim for the
// given name and namespace
func (s *DataStore) DeletePersistentVolumeClaim(ns, pvcName string) error {
	return s.kubeClient.CoreV1().PersistentVolumeClaims(ns).Delete(context.TODO(), pvcName, metav1.DeleteOptions{})
}

// UpdatePersistentVolumeClaim expand the PersistentVolumeClaim from the
// index for the given name and namespace
func (s *DataStore) UpdatePersistentVolumeClaim(namespace string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return s.kubeClient.CoreV1().PersistentVolumeClaims(namespace).Update(context.TODO(), pvc, metav1.UpdateOptions{})
}

// GetPersistentVolumeClaimRO gets the PersistentVolumeClaim from the
// index for the given name and namespace
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetPersistentVolumeClaimRO(namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	return s.persistentVolumeClaimLister.PersistentVolumeClaims(namespace).Get(pvcName)
}

// GetPersistentVolumeClaim gets a mutable PersistentVolumeClaim for the given name and namespace
func (s *DataStore) GetPersistentVolumeClaim(namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	resultRO, err := s.persistentVolumeClaimLister.PersistentVolumeClaims(namespace).Get(pvcName)
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// ListVolumeAttachmentsRO gets a list of volumeattachments
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) ListVolumeAttachmentsRO() ([]*storagev1.VolumeAttachment, error) {
	return s.volumeAttachmentLister.List(labels.Everything())
}

// CreateConfigMap creates a ConfigMap resource
func (s *DataStore) CreateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return s.kubeClient.CoreV1().ConfigMaps(s.namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
}

// UpdateConfigMap updates ConfigMap resource
func (s *DataStore) UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return s.kubeClient.CoreV1().ConfigMaps(s.namespace).Update(context.TODO(), configMap, metav1.UpdateOptions{})
}

// GetConfigMapRO gets ConfigMap with the given name in s.namespace
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetConfigMapRO(namespace, name string) (*corev1.ConfigMap, error) {
	if namespace == s.namespace {
		return s.configMapLister.ConfigMaps(namespace).Get(name)
	}
	return s.kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// GetConfigMap return a new ConfigMap object for the given namespace and name
func (s *DataStore) GetConfigMap(namespace, name string) (resultRO *corev1.ConfigMap, err error) {
	if namespace == s.namespace {
		resultRO, err = s.configMapLister.ConfigMaps(namespace).Get(name)
	} else {
		resultRO, err = s.kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// DeleteConfigMap deletes the ConfigMap for the given name and namespace
func (s *DataStore) DeleteConfigMap(namespace, name string) error {
	err := s.kubeClient.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// GetSecretRO gets Secret with the given namespace and name
// This function returns direct reference to the internal cache object and should not be mutated.
// Consider using this function when you can guarantee read only access and don't want the overhead of deep copies
func (s *DataStore) GetSecretRO(namespace, name string) (*corev1.Secret, error) {
	if namespace == s.namespace {
		return s.secretLister.Secrets(namespace).Get(name)
	}
	return s.kubeClient.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// GetSecret return a new Secret object with the given namespace and name
func (s *DataStore) GetSecret(namespace, name string) (resultRO *corev1.Secret, err error) {
	if namespace == s.namespace {
		resultRO, err = s.secretLister.Secrets(namespace).Get(name)
	} else {
		resultRO, err = s.kubeClient.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}
	// Cannot use cached object from lister
	return resultRO.DeepCopy(), nil
}

// UpdateSecret updates the Secret resource with the given object and namespace
func (s *DataStore) UpdateSecret(namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	return s.kubeClient.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
}

// GetPriorityClass gets the PriorityClass from the index for the
// given name
func (s *DataStore) GetPriorityClass(pcName string) (*schedulingv1.PriorityClass, error) {
	return s.priorityClassLister.Get(pcName)
}

// GetPodContainerLogRequest returns the Pod log for the given pod name,
// container name and namespace
func (s *DataStore) GetPodContainerLogRequest(podName, containerName string) *rest.Request {
	return s.kubeClient.CoreV1().Pods(s.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:  containerName,
		Timestamps: true,
	})
}

// GetKubernetesVersion returns the server version
func (s *DataStore) GetKubernetesVersion() (*version.Info, error) {
	return s.kubeClient.Discovery().ServerVersion()
}

// CreateService creates a Service resource
// for the given CreateService object and namespace
func (s *DataStore) CreateService(ns string, service *corev1.Service) (*corev1.Service, error) {
	return s.kubeClient.CoreV1().Services(ns).Create(context.TODO(), service, metav1.CreateOptions{})
}

// GetService gets the Service for the given name and namespace
func (s *DataStore) GetService(namespace, name string) (*corev1.Service, error) {
	if namespace == s.namespace {
		return s.serviceLister.Services(namespace).Get(name)
	}
	return s.kubeClient.CoreV1().Services(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

// DeleteService deletes the Service for the given name and namespace
func (s *DataStore) DeleteService(namespace, name string) error {
	return s.kubeClient.CoreV1().Services(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// UpdateService updates the Service resource with the given object and namespace
func (s *DataStore) UpdateService(namespace string, service *corev1.Service) (*corev1.Service, error) {
	return s.kubeClient.CoreV1().Services(namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
}

// NewPVManifestForVolume returns a new PersistentVolume object for a longhorn volume
func NewPVManifestForVolume(v *longhorn.Volume, pvName, storageClassName, fsType string) *corev1.PersistentVolume {
	diskSelector := strings.Join(v.Spec.DiskSelector, ",")
	nodeSelector := strings.Join(v.Spec.NodeSelector, ",")

	volAttributes := map[string]string{
		"diskSelector":        diskSelector,
		"nodeSelector":        nodeSelector,
		"numberOfReplicas":    strconv.Itoa(v.Spec.NumberOfReplicas),
		"staleReplicaTimeout": strconv.Itoa(v.Spec.StaleReplicaTimeout),
	}

	if v.Spec.Encrypted {
		volAttributes["encrypted"] = strconv.FormatBool(v.Spec.Encrypted)
	}

	accessMode := corev1.ReadWriteOnce
	if v.Spec.AccessMode == longhorn.AccessModeReadWriteMany {
		accessMode = corev1.ReadWriteMany
		volAttributes["migratable"] = strconv.FormatBool(v.Spec.Migratable)
	}

	return NewPVManifest(v.Spec.Size, pvName, v.Name, storageClassName, fsType, volAttributes, accessMode)
}

// NewPVManifest returns a new PersistentVolume object
func NewPVManifest(size int64, pvName, volumeName, storageClassName, fsType string, volAttributes map[string]string, accessMode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolume {
	defaultVolumeMode := corev1.PersistentVolumeFilesystem
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: *resource.NewQuantity(size, resource.BinarySI),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				accessMode,
			},

			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,

			VolumeMode: &defaultVolumeMode,

			StorageClassName: storageClassName,

			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           types.LonghornDriverName,
					FSType:           fsType,
					VolumeHandle:     volumeName,
					VolumeAttributes: volAttributes,
				},
			},
		},
	}
}

// NewPVCManifestForVolume returns a new PersistentVolumeClaim object for a longhorn volume
func NewPVCManifestForVolume(v *longhorn.Volume, pvName, ns, pvcName, storageClassName string) *corev1.PersistentVolumeClaim {
	accessMode := corev1.ReadWriteOnce
	if v.Spec.AccessMode == longhorn.AccessModeReadWriteMany {
		accessMode = corev1.ReadWriteMany
	}

	return NewPVCManifest(v.Spec.Size, pvName, ns, pvcName, storageClassName, accessMode)
}

// NewPVCManifest returns a new PersistentVolumeClaim object
func NewPVCManifest(size int64, pvName, ns, pvcName, storageClassName string, accessMode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ns,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				accessMode,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *resource.NewQuantity(size, resource.BinarySI),
				},
			},
			StorageClassName: &storageClassName,
			VolumeName:       pvName,
		},
	}
}

// GetStorageIPFromPod returns the given pod network-status IP of the name matching the storage-network setting value.
// If the storage-network setting is empty or encountered an error, return the pod IP instead.
// For below example, given "kube-system/demo-192-168-0-0" will return "192.168.1.175".
//
// apiVersion: v1
// kind: Pod
// metadata:
//
//	annotations:
//	  k8s.v1.cni.cncf.io/network-status: |-
//	    [{
//	  	  "name": "cbr0",
//	  	  "interface": "eth0",
//	  	  "ips": [
//	  		  "10.42.0.175"
//	  	  ],
//	  	  "mac": "be:67:b2:19:17:84",
//	  	  "default": true,
//	  	  "dns": {}
//	    },{
//	  	  "name": "kube-system/demo-192-168-0-0",
//	  	  "interface": "lhnet1",
//	  	  "ips": [
//	  		  "192.168.1.175"
//	  	  ],
//	  	  "mac": "02:59:e5:d4:ae:ea",
//	  	  "dns": {}
//	    }]
func (s *DataStore) GetStorageIPFromPod(pod *corev1.Pod) string {
	storageNetwork, err := s.GetSetting(types.SettingNameStorageNetwork)
	if err != nil {
		logrus.Warnf("Failed to get %v setting, use %v pod IP %v", types.SettingNameStorageNetwork, pod.Name, pod.Status.PodIP)
		return pod.Status.PodIP
	}

	if storageNetwork.Value == types.CniNetworkNone {
		logrus.Tracef("Found %v setting is empty, use %v pod IP %v", types.SettingNameStorageNetwork, pod.Name, pod.Status.PodIP)
		return pod.Status.PodIP
	}

	// Check if the network-status annotation exists.
	status, ok := pod.Annotations[string(types.CNIAnnotationNetworkStatus)]
	if !ok {
		// If the network-status annotation is missing, check the deprecated annotation.
		logrus.Debugf("Missing %v annotation, checking deprecated %v annotation", types.CNIAnnotationNetworkStatus, types.CNIAnnotationNetworksStatus)
		status, ok = pod.Annotations[string(types.CNIAnnotationNetworksStatus)]

		// If the deprecated annotation is also missing, use the pod IP.
		if !ok {
			logrus.Warnf("Missing %v annotation, use %v pod IP %v", types.CNIAnnotationNetworkStatus, pod.Name, pod.Status.PodIP)
			return pod.Status.PodIP
		}
	}

	nets := []types.CniNetwork{}
	err = json.Unmarshal([]byte(status), &nets)
	if err != nil {
		logrus.Warnf("Failed to unmarshal %v annotation, use %v pod IP %v", types.CNIAnnotationNetworkStatus, pod.Name, pod.Status.PodIP)
		return pod.Status.PodIP
	}

	for _, net := range nets {
		if net.Name != storageNetwork.Value {
			continue
		}

		sort.Strings(net.IPs)
		if net.IPs != nil {
			return net.IPs[0]
		}
	}

	logrus.Warnf("Failed to get storage IP from %v pod, use IP %v", pod.Name, pod.Status.PodIP)
	return pod.Status.PodIP
}

func (s *DataStore) UpdatePVAnnotation(volume *longhorn.Volume, annotationKey, annotationVal string) error {
	pv, err := s.GetPersistentVolume(volume.Status.KubernetesStatus.PVName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if pv.Annotations == nil {
		pv.Annotations = map[string]string{}
	}

	val, ok := pv.Annotations[annotationKey]
	if ok && val == annotationVal {
		return nil
	}

	pv.Annotations[annotationKey] = annotationVal

	_, err = s.UpdatePersistentVolume(pv)

	return err
}

// CreateJob creates a Job resource for the given job object in the Longhorn namespace
func (s *DataStore) CreateJob(job *batchv1.Job) (*batchv1.Job, error) {
	return s.kubeClient.BatchV1().Jobs(s.namespace).Create(context.TODO(), job, metav1.CreateOptions{})
}

// DeleteJob delete a Job resource for the given job name in the Longhorn namespace
func (s *DataStore) DeleteJob(name string) error {
	propagation := metav1.DeletePropagationForeground
	return s.kubeClient.BatchV1().Jobs(s.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

// GetJob get a Job resource for the given job name in the Longhorn namespace
func (s *DataStore) GetJob(name string) (*batchv1.Job, error) {
	return s.kubeClient.BatchV1().Jobs(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// CreateServiceAccount create a ServiceAccount resource with the given ServiceAccount object in the Longhorn
// namespace
func (s *DataStore) CreateServiceAccount(serviceAccount *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	return s.kubeClient.CoreV1().ServiceAccounts(s.namespace).Create(context.TODO(), serviceAccount, metav1.CreateOptions{})
}

// GetServiceAccount get the ServiceAccount resource of the given name in the Longhorn namespace
func (s *DataStore) GetServiceAccount(name string) (*corev1.ServiceAccount, error) {
	return s.kubeClient.CoreV1().ServiceAccounts(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateServiceAccount updates the ServiceAccount resource with the given ServiceAccount object in the Longhorn
// namespace
func (s *DataStore) UpdateServiceAccount(serviceAccount *corev1.ServiceAccount) (*corev1.ServiceAccount, error) {
	return s.kubeClient.CoreV1().ServiceAccounts(s.namespace).Update(context.TODO(), serviceAccount, metav1.UpdateOptions{})
}

// CreateClusterRole create a ClusterRole resource with the given ClusterRole object
func (s *DataStore) CreateClusterRole(clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	return s.kubeClient.RbacV1().ClusterRoles().Create(context.TODO(), clusterRole, metav1.CreateOptions{})
}

// GetClusterRole get the ClusterRole resource of the given name
func (s *DataStore) GetClusterRole(name string) (*rbacv1.ClusterRole, error) {
	return s.kubeClient.RbacV1().ClusterRoles().Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateClusterRole create a ClusterRole resource with the given ClusterRole objecct
func (s *DataStore) UpdateClusterRole(clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	return s.kubeClient.RbacV1().ClusterRoles().Update(context.TODO(), clusterRole, metav1.UpdateOptions{})
}

// CreateClusterRoleBinding create a ClusterRoleBinding resource with the given ClusterRoleBinding object
func (s *DataStore) CreateClusterRoleBinding(clusterRoleBinding *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	return s.kubeClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
}

// GetClusterRoleBinding get the ClusterRoleBinding resource of the given name
func (s *DataStore) GetClusterRoleBinding(name string) (*rbacv1.ClusterRoleBinding, error) {
	return s.kubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateClusterRoleBinding updates the ClusterRoleBinding resource with the given ClusterRoleBinding object
func (s *DataStore) UpdateClusterRoleBinding(clusterRoleBinding *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	return s.kubeClient.RbacV1().ClusterRoleBindings().Update(context.TODO(), clusterRoleBinding, metav1.UpdateOptions{})
}

// CreatePodSecurityPolicy create a PodSecurityPolicy resource with the given PodSecurityPolicy object
func (s *DataStore) CreatePodSecurityPolicy(podSecurityPolicy *policyv1beta1.PodSecurityPolicy) (*policyv1beta1.PodSecurityPolicy, error) {
	return s.kubeClient.PolicyV1beta1().PodSecurityPolicies().Create(context.TODO(), podSecurityPolicy, metav1.CreateOptions{})
}

// GetPodSecurityPolicy get the PodSecurityPolicy resource of the given name
func (s *DataStore) GetPodSecurityPolicy(name string) (*policyv1beta1.PodSecurityPolicy, error) {
	return s.kubeClient.PolicyV1beta1().PodSecurityPolicies().Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdatePodSecurityPolicy updates the PodSecurityPolicy resource with the given PodSecurityPolicy object
func (s *DataStore) UpdatePodSecurityPolicy(podSecurityPolicy *policyv1beta1.PodSecurityPolicy) (*policyv1beta1.PodSecurityPolicy, error) {
	return s.kubeClient.PolicyV1beta1().PodSecurityPolicies().Update(context.TODO(), podSecurityPolicy, metav1.UpdateOptions{})
}

// CreateRole create a Role resource with the given Role object in the Longhorn namespace
func (s *DataStore) CreateRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	return s.kubeClient.RbacV1().Roles(s.namespace).Create(context.TODO(), role, metav1.CreateOptions{})
}

// GetRole get the Role resource of the given name in the Longhorn namespace
func (s *DataStore) GetRole(name string) (*rbacv1.Role, error) {
	return s.kubeClient.RbacV1().Roles(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateRole updates the Role resource with the given Role object in the Longhorn namespace
func (s *DataStore) UpdateRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	return s.kubeClient.RbacV1().Roles(s.namespace).Update(context.TODO(), role, metav1.UpdateOptions{})
}

// CreateRoleBinding create a RoleBinding resource with the given RoleBinding object in the Longhorn namespace
func (s *DataStore) CreateRoleBinding(role *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	return s.kubeClient.RbacV1().RoleBindings(s.namespace).Create(context.TODO(), role, metav1.CreateOptions{})
}

// GetRoleBinding get the RoleBinding resource of the given name in the Longhorn namespace
func (s *DataStore) GetRoleBinding(name string) (*rbacv1.RoleBinding, error) {
	return s.kubeClient.RbacV1().RoleBindings(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// UpdateRoleBinding updates the RoleBinding resource with the given RoleBinding object in the Longhorn namespace
func (s *DataStore) UpdateRoleBinding(roleBinding *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	return s.kubeClient.RbacV1().RoleBindings(s.namespace).Update(context.TODO(), roleBinding, metav1.UpdateOptions{})
}

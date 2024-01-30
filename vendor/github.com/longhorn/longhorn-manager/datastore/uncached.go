package datastore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhornapis "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// GetLonghornEventList returns an uncached list of longhorn events for the
// given namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetLonghornEventList() (*corev1.EventList, error) {
	return s.kubeClient.CoreV1().Events(s.namespace).List(context.TODO(), metav1.ListOptions{FieldSelector: "involvedObject.apiVersion=longhorn.io/v1beta2"})
}

// GetAllPodsList returns an uncached list of pods for the given namespace
// directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllPodsList() (runtime.Object, error) {
	return s.kubeClient.CoreV1().Pods(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllServicesList returns an uncached list of services for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllServicesList() (runtime.Object, error) {
	return s.kubeClient.CoreV1().Services(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllServiceAccountList returns an uncached list of ServiceAccounts in
// Longhorn namespace directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllServiceAccountList() (runtime.Object, error) {
	return s.kubeClient.CoreV1().ServiceAccounts(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllDeploymentsList returns an uncached list of deployments for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllDeploymentsList() (runtime.Object, error) {
	return s.kubeClient.AppsV1().Deployments(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllDaemonSetsList returns an uncached list of daemonsets for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllDaemonSetsList() (runtime.Object, error) {
	return s.kubeClient.AppsV1().DaemonSets(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllStatefulSetsList returns an uncached list of statefulsets for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllStatefulSetsList() (runtime.Object, error) {
	return s.kubeClient.AppsV1().StatefulSets(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllJobsList returns an uncached list of jobs for the given namespace
// directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllJobsList() (runtime.Object, error) {
	return s.kubeClient.BatchV1().Jobs(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllCronJobsList returns an uncached list of cronjobs for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllCronJobsList() (runtime.Object, error) {
	return s.kubeClient.BatchV1().CronJobs(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllNodesList returns an uncached list of nodes for the given namespace
// directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllNodesList() (runtime.Object, error) {
	return s.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
}

// GetLonghornNamespace returns an uncached namespace object for the given
// Longhorn namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetLonghornNamespace() (*corev1.Namespace, error) {
	return s.kubeClient.CoreV1().Namespaces().Get(context.TODO(), s.namespace, metav1.GetOptions{})
}

// GetNamespace returns an uncached namespace object for the given namespace
// directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetNamespace(name string) (*corev1.Namespace, error) {
	return s.kubeClient.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
}

// GetAllEventsList returns an uncached list of events for the given namespace
// directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllEventsList() (runtime.Object, error) {
	return s.kubeClient.CoreV1().Events(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllConfigMaps returns an uncached list of configmaps for the given
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllConfigMaps() (runtime.Object, error) {
	return s.kubeClient.CoreV1().ConfigMaps(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllVolumeAttachments returns an uncached list of volumeattachments for
// the given namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetAllVolumeAttachments() (runtime.Object, error) {
	return s.kubeClient.StorageV1().VolumeAttachments().List(context.TODO(), metav1.ListOptions{})
}

// GetAllClusterRoleList returns an uncached list of ClusterRoles directly from
// the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllClusterRoleList() (runtime.Object, error) {
	return s.kubeClient.RbacV1().ClusterRoles().List(context.TODO(), metav1.ListOptions{})
}

// GetAllClusterRoleBindingList returns an uncached list of ClusterRoleBindings
// directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllClusterRoleBindingList() (runtime.Object, error) {
	return s.kubeClient.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
}

// GetAllRoleList returns an uncached list of Roles directly from the API
// server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllRoleList() (runtime.Object, error) {
	return s.kubeClient.RbacV1().Roles(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllRoleBindingList returns an uncached list of RoleBindings directly from
// the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllRoleBindingList() (runtime.Object, error) {
	return s.kubeClient.RbacV1().RoleBindings(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllPodSecurityPolicyList returns an uncached list of PodSecurityPolicys
// directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllPodSecurityPolicyList() (runtime.Object, error) {
	return s.kubeClient.PolicyV1beta1().PodSecurityPolicies().List(context.TODO(), metav1.ListOptions{})
}

// GetAllLonghornStorageClassList returns an uncached list of Longhorn
// StorageClasses directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornStorageClassList() (runtime.Object, error) {
	scList, err := s.kubeClient.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	longhornStorageClasses := []storagev1.StorageClass{}
	for _, sc := range scList.Items {
		if sc.Provisioner != types.LonghornDriverName {
			continue
		}
		longhornStorageClasses = append(longhornStorageClasses, sc)
	}

	scList.Items = longhornStorageClasses
	return scList, nil
}

// GetAllPersistentVolumeClaimsByPersistentVolumeProvisioner returns an uncached
// list of PersistentVolumeClaims with the StorageClass of the PersistentVolumes
// with provisioner "driver.longhorn.io".
// This is directly from the API server. Using cached informers should be
// preferred but current lister doesn't have a field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllPersistentVolumeClaimsByPersistentVolumeProvisioner() (runtime.Object, error) {
	pvcList, err := s.kubeClient.CoreV1().PersistentVolumeClaims("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	storageClassNames, err := s.ListStorageClassesInPersistentVolumesWithLonghornProvisioner()
	if err != nil {
		return nil, err
	}

	storageClassPVCs := []corev1.PersistentVolumeClaim{}
	for _, pvc := range pvcList.Items {
		if pvc.Spec.StorageClassName == nil {
			continue
		}

		if !util.Contains(storageClassNames, *pvc.Spec.StorageClassName) {
			continue
		}

		storageClassPVCs = append(storageClassPVCs, pvc)
	}

	pvcList.Items = storageClassPVCs
	return pvcList, nil
}

// GetAllPersistentVolumeClaimsByStorageClass returns an uncached list of
// PersistentVolumeClaims of the given storage class name directly from the API
// server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllPersistentVolumeClaimsByStorageClass(storageClassNames []string) (runtime.Object, error) {
	pvcList, err := s.kubeClient.CoreV1().PersistentVolumeClaims("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	storageClassPVCs := []corev1.PersistentVolumeClaim{}
	for _, pvc := range pvcList.Items {
		if pvc.Spec.StorageClassName == nil {
			continue
		}

		if !util.Contains(storageClassNames, *pvc.Spec.StorageClassName) {
			continue
		}

		storageClassPVCs = append(storageClassPVCs, pvc)
	}

	pvcList.Items = storageClassPVCs
	return pvcList, nil
}

// GetAllPersistentVolumesWithLonghornProvisioner returns an uncached list of
// PersistentVolumes with provisioner "driver.longhorn.io" directly from the API
// server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllPersistentVolumesWithLonghornProvisioner() (runtime.Object, error) {
	pvList, err := s.kubeClient.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	longhornPVs := []corev1.PersistentVolume{}
	for _, pv := range pvList.Items {
		if pv.Spec.CSI == nil {
			continue
		}
		if pv.Spec.CSI.Driver != types.LonghornDriverName {
			continue
		}

		longhornPVs = append(longhornPVs, pv)
	}

	pvList.Items = longhornPVs
	return pvList, nil
}

// GetAllLonghornSettings returns an uncached list of Settings in Longhorn
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornSettings() (runtime.Object, error) {
	return s.lhClient.LonghornV1beta2().Settings(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllLonghornEngineImages returns an uncached list of EngineImages in
// Longhorn namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornEngineImages() (runtime.Object, error) {
	return s.lhClient.LonghornV1beta2().EngineImages(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetLonghornEngineImage returns the uncached EngineImage in Longhorn
// namespace directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetLonghornEngineImage(name string) (runtime.Object, error) {
	return s.lhClient.LonghornV1beta2().EngineImages(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// GetAllLonghornVolumes returns an uncached list of Volumes in Longhorn
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornVolumes() (runtime.Object, error) {
	return s.lhClient.LonghornV1beta2().Volumes(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// ListVolumeEnginesUncached returns an uncached list of Volume's engines in
// Longhorn namespace directly from the API server.
func (s *DataStore) ListVolumeEnginesUncached(volumeName string) ([]longhorn.Engine, error) {
	engineList, err := s.lhClient.LonghornV1beta2().Engines(s.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.Set(types.GetVolumeLabels(volumeName)).String(),
	})
	if err != nil {
		return nil, err
	}
	return engineList.Items, nil
}

// GetAllLonghornRecurringJobs returns an uncached list of RecurringJobs in
// Longhorn namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornRecurringJobs() (runtime.Object, error) {
	return s.lhClient.LonghornV1beta2().RecurringJobs(s.namespace).List(context.TODO(), metav1.ListOptions{})
}

// GetAllLonghornCustomResourceDefinitions returns an uncached list of Longhorn grouped
// CustomResourceDefinitions directly from the API server.
// Direct retrieval from the API server should only be used for one-shot tasks.
func (s *DataStore) GetAllLonghornCustomResourceDefinitions() (runtime.Object, error) {
	crdList, err := s.extensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	longhornCRDs := []apiextensionsv1.CustomResourceDefinition{}
	for _, crd := range crdList.Items {
		if crd.Spec.Group != longhornapis.GroupName {
			continue
		}

		longhornCRDs = append(longhornCRDs, crd)
	}

	crdList.Items = longhornCRDs
	return crdList, nil
}

// GetConfigMapWithoutCache return a new ConfigMap via Kubernetes client object for the given namespace and name
func (s *DataStore) GetConfigMapWithoutCache(namespace, name string) (*corev1.ConfigMap, error) {
	return s.kubeClient.CoreV1().ConfigMaps(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

// GetLonghornSnapshotUncached returns the uncached Snapshot in the Longhorn namespace directly from the API server.
// Direct retrieval from the API server should ideally only be used for one-shot tasks, but there may be other limited
// situations in which it is necessary.
func (s *DataStore) GetLonghornSnapshotUncached(name string) (*longhorn.Snapshot, error) {
	return s.lhClient.LonghornV1beta2().Snapshots(s.namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

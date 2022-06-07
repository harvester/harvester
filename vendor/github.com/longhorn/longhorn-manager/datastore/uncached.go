package datastore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	return s.kubeClient.BatchV1beta1().CronJobs(s.namespace).List(context.TODO(), metav1.ListOptions{})
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
// namespace directly from the API server.
// Using cached informers should be preferred but current lister doesn't have a
// field selector.
// Direct retrieval from the API server should only be used for one-shot tasks.
// For example, support bundle creation
func (s *DataStore) GetLonghornNamespace() (*corev1.Namespace, error) {
	return s.kubeClient.CoreV1().Namespaces().Get(context.TODO(), s.namespace, metav1.GetOptions{})
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

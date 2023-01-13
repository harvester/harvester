package fakeclients

import (
	"context"

	loggingv1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	loggingv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
)

type ClusterFlowClient func() loggingv1type.ClusterFlowInterface

func (c ClusterFlowClient) Create(clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	clusterFlow.Namespace = ""
	return c().Create(context.TODO(), clusterFlow, metav1.CreateOptions{})
}
func (c ClusterFlowClient) Update(clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowClient) UpdateStatus(clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, metav1.DeleteOptions{})
}
func (c ClusterFlowClient) Get(namespace, name string, options metav1.GetOptions) (*loggingv1.ClusterFlow, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c ClusterFlowClient) List(namespace string, opts metav1.ListOptions) (*loggingv1.ClusterFlowList, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *loggingv1.ClusterFlow, err error) {
	panic("implement me")
}

type ClusterFlowCache func() loggingv1type.ClusterFlowInterface

func (c ClusterFlowCache) Get(namespace, name string) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowCache) List(namespace string, selector labels.Selector) ([]*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowCache) AddIndexer(indexName string, indexer ctlloggingv1.ClusterFlowIndexer) {
	panic("implement me")
}
func (c ClusterFlowCache) GetByIndex(indexName, key string) ([]*loggingv1.ClusterFlow, error) {
	panic("implement me")
}

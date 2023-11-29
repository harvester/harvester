package fakeclients

import (
	"context"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	loggingv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
)

type ClusterOutputClient func() loggingv1type.ClusterOutputInterface

func (c ClusterOutputClient) Create(clusterOutput *loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	clusterOutput.Namespace = ""
	return c().Create(context.TODO(), clusterOutput, metav1.CreateOptions{})
}
func (c ClusterOutputClient) Update(clusterOutput *loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputClient) UpdateStatus(clusterOutput *loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, metav1.DeleteOptions{})
}
func (c ClusterOutputClient) Get(namespace, name string, options metav1.GetOptions) (*loggingv1.ClusterOutput, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c ClusterOutputClient) List(namespace string, opts metav1.ListOptions) (*loggingv1.ClusterOutputList, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *loggingv1.ClusterOutput, err error) {
	panic("implement me")
}

type ClusterOutputCache func() loggingv1type.ClusterOutputInterface

func (c ClusterOutputCache) Get(namespace, name string) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputCache) List(namespace string, selector labels.Selector) ([]*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputCache) AddIndexer(indexName string, indexer ctlloggingv1.ClusterOutputIndexer) {
	panic("implement me")
}
func (c ClusterOutputCache) GetByIndex(indexName, key string) ([]*loggingv1.ClusterOutput, error) {
	panic("implement me")
}

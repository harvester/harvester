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

type LoggingClient func() loggingv1type.LoggingInterface

func (c LoggingClient) Create(logging *loggingv1.Logging) (*loggingv1.Logging, error) {
	return c().Create(context.TODO(), logging, metav1.CreateOptions{})
}
func (c LoggingClient) Update(logging *loggingv1.Logging) (*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingClient) UpdateStatus(logging *loggingv1.Logging) (*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, metav1.DeleteOptions{})
}
func (c LoggingClient) Get(name string, options metav1.GetOptions) (*loggingv1.Logging, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c LoggingClient) List(opts metav1.ListOptions) (*loggingv1.LoggingList, error) {
	panic("implement me")
}

func (c LoggingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c LoggingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *loggingv1.Logging, err error) {
	panic("implement me")
}

type LoggingCache func() loggingv1type.LoggingInterface

func (c LoggingCache) Get(name string) (*loggingv1.Logging, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c LoggingCache) List(selector labels.Selector) ([]*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingCache) AddIndexer(indexName string, indexer ctlloggingv1.LoggingIndexer) {
	panic("implement me")
}
func (c LoggingCache) GetByIndex(indexName, key string) ([]*loggingv1.Logging, error) {
	panic("implement me")
}

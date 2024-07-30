package fakeclients

import (
	"context"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	loggingv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/logging.banzaicloud.io/v1beta1"
)

type LoggingClient func() loggingv1type.LoggingInterface

func (c LoggingClient) Create(logging *loggingv1.Logging) (*loggingv1.Logging, error) {
	return c().Create(context.TODO(), logging, metav1.CreateOptions{})
}
func (c LoggingClient) Update(_ *loggingv1.Logging) (*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingClient) UpdateStatus(_ *loggingv1.Logging) (*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c LoggingClient) Get(name string, options metav1.GetOptions) (*loggingv1.Logging, error) {
	return c().Get(context.TODO(), name, options)
}
func (c LoggingClient) List(_ metav1.ListOptions) (*loggingv1.LoggingList, error) {
	panic("implement me")
}

func (c LoggingClient) Watch(_ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c LoggingClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (result *loggingv1.Logging, err error) {
	panic("implement me")
}
func (c LoggingClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*loggingv1.Logging, *loggingv1.LoggingList], error) {
	panic("implement me")
}

type LoggingCache func() loggingv1type.LoggingInterface

func (c LoggingCache) Get(name string) (*loggingv1.Logging, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c LoggingCache) List(_ labels.Selector) ([]*loggingv1.Logging, error) {
	panic("implement me")
}
func (c LoggingCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.Logging]) {
	panic("implement me")
}
func (c LoggingCache) GetByIndex(_, _ string) ([]*loggingv1.Logging, error) {
	panic("implement me")
}

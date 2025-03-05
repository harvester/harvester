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

type FluentbitAgentClient func() loggingv1type.FluentbitAgentInterface

func (c FluentbitAgentClient) Create(fluentbitAgent *loggingv1.FluentbitAgent) (*loggingv1.FluentbitAgent, error) {
	return c().Create(context.TODO(), fluentbitAgent, metav1.CreateOptions{})
}
func (c FluentbitAgentClient) Update(_ *loggingv1.FluentbitAgent) (*loggingv1.FluentbitAgent, error) {
	panic("implement me")
}
func (c FluentbitAgentClient) UpdateStatus(_ *loggingv1.FluentbitAgent) (*loggingv1.FluentbitAgent, error) {
	panic("implement me")
}
func (c FluentbitAgentClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c FluentbitAgentClient) Get(name string, options metav1.GetOptions) (*loggingv1.FluentbitAgent, error) {
	return c().Get(context.TODO(), name, options)
}
func (c FluentbitAgentClient) List(_ metav1.ListOptions) (*loggingv1.FluentbitAgentList, error) {
	panic("implement me")
}

func (c FluentbitAgentClient) Watch(_ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c FluentbitAgentClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (result *loggingv1.FluentbitAgent, err error) {
	panic("implement me")
}
func (c FluentbitAgentClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*loggingv1.FluentbitAgent, *loggingv1.FluentbitAgentList], error) {
	panic("implement me")
}

type FluentbitAgentCache func() loggingv1type.FluentbitAgentInterface

func (c FluentbitAgentCache) Get(name string) (*loggingv1.FluentbitAgent, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}
func (c FluentbitAgentCache) List(_ labels.Selector) ([]*loggingv1.FluentbitAgent, error) {
	panic("implement me")
}
func (c FluentbitAgentCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.FluentbitAgent]) {
	panic("implement me")
}
func (c FluentbitAgentCache) GetByIndex(_, _ string) ([]*loggingv1.FluentbitAgent, error) {
	panic("implement me")
}

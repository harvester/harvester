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

type FlowClient func() loggingv1type.FlowInterface

func (c FlowClient) Create(Flow *loggingv1.Flow) (*loggingv1.Flow, error) {
	Flow.Namespace = ""
	return c().Create(context.TODO(), Flow, metav1.CreateOptions{})
}
func (c FlowClient) Update(*loggingv1.Flow) (*loggingv1.Flow, error) {
	panic("implement me")
}
func (c FlowClient) UpdateStatus(*loggingv1.Flow) (*loggingv1.Flow, error) {
	panic("implement me")
}
func (c FlowClient) Delete(_, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c FlowClient) Get(_, name string, options metav1.GetOptions) (*loggingv1.Flow, error) {
	return c().Get(context.TODO(), name, options)
}
func (c FlowClient) List(_ string, _ metav1.ListOptions) (*loggingv1.FlowList, error) {
	panic("implement me")
}
func (c FlowClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c FlowClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *loggingv1.Flow, err error) {
	panic("implement me")
}
func (c FlowClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*loggingv1.Flow, *loggingv1.FlowList], error) {
	panic("implement me")
}

type FlowCache func() loggingv1type.FlowInterface

func (c FlowCache) Get(_, _ string) (*loggingv1.Flow, error) {
	panic("implement me")
}
func (c FlowCache) List(namespace string, selector labels.Selector) ([]*loggingv1.Flow, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*loggingv1.Flow, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Namespace == namespace {
			result = append(result, &list.Items[i])
		}
	}
	return result, err
}
func (c FlowCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.Flow]) {
	panic("implement me")
}
func (c FlowCache) GetByIndex(_, _ string) ([]*loggingv1.Flow, error) {
	panic("implement me")
}

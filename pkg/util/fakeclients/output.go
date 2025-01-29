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

type OutputClient func() loggingv1type.OutputInterface

func (c OutputClient) Create(Output *loggingv1.Output) (*loggingv1.Output, error) {
	Output.Namespace = ""
	return c().Create(context.TODO(), Output, metav1.CreateOptions{})
}
func (c OutputClient) Update(*loggingv1.Output) (*loggingv1.Output, error) {
	panic("implement me")
}
func (c OutputClient) UpdateStatus(*loggingv1.Output) (*loggingv1.Output, error) {
	panic("implement me")
}
func (c OutputClient) Delete(_, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c OutputClient) Get(_, name string, options metav1.GetOptions) (*loggingv1.Output, error) {
	return c().Get(context.TODO(), name, options)
}
func (c OutputClient) List(_ string, _ metav1.ListOptions) (*loggingv1.OutputList, error) {
	panic("implement me")
}
func (c OutputClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c OutputClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*loggingv1.Output, error) {
	panic("implement me")
}
func (c OutputClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*loggingv1.Output, *loggingv1.OutputList], error) {
	panic("implement me")
}

type OutputCache func() loggingv1type.OutputInterface

func (c OutputCache) Get(_, _ string) (*loggingv1.Output, error) {
	panic("implement me")
}
func (c OutputCache) List(namespace string, selector labels.Selector) ([]*loggingv1.Output, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*loggingv1.Output, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Namespace == namespace {
			result = append(result, &list.Items[i])
		}
	}
	return result, err
}
func (c OutputCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.Output]) {
	panic("implement me")
}
func (c OutputCache) GetByIndex(_, _ string) ([]*loggingv1.Output, error) {
	panic("implement me")
}

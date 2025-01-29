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

type ClusterFlowClient func() loggingv1type.ClusterFlowInterface

func (c ClusterFlowClient) Create(clusterFlow *loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	clusterFlow.Namespace = ""
	return c().Create(context.TODO(), clusterFlow, metav1.CreateOptions{})
}
func (c ClusterFlowClient) Update(*loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowClient) UpdateStatus(*loggingv1.ClusterFlow) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Delete(_, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c ClusterFlowClient) Get(_, name string, options metav1.GetOptions) (*loggingv1.ClusterFlow, error) {
	return c().Get(context.TODO(), name, options)
}
func (c ClusterFlowClient) List(_ string, _ metav1.ListOptions) (*loggingv1.ClusterFlowList, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c ClusterFlowClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *loggingv1.ClusterFlow, err error) {
	panic("implement me")
}
func (c ClusterFlowClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*loggingv1.ClusterFlow, *loggingv1.ClusterFlowList], error) {
	panic("implement me")
}

type ClusterFlowCache func() loggingv1type.ClusterFlowInterface

func (c ClusterFlowCache) Get(_, _ string) (*loggingv1.ClusterFlow, error) {
	panic("implement me")
}
func (c ClusterFlowCache) List(namespace string, selector labels.Selector) ([]*loggingv1.ClusterFlow, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*loggingv1.ClusterFlow, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Namespace == namespace {
			result = append(result, &list.Items[i])
		}
	}
	return result, err
}
func (c ClusterFlowCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.ClusterFlow]) {
	panic("implement me")
}
func (c ClusterFlowCache) GetByIndex(_, _ string) ([]*loggingv1.ClusterFlow, error) {
	panic("implement me")
}

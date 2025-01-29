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

type ClusterOutputClient func() loggingv1type.ClusterOutputInterface

func (c ClusterOutputClient) Create(ClusterOutput *loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	ClusterOutput.Namespace = ""
	return c().Create(context.TODO(), ClusterOutput, metav1.CreateOptions{})
}
func (c ClusterOutputClient) Update(*loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputClient) UpdateStatus(*loggingv1.ClusterOutput) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Delete(_, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}
func (c ClusterOutputClient) Get(_, name string, options metav1.GetOptions) (*loggingv1.ClusterOutput, error) {
	return c().Get(context.TODO(), name, options)
}
func (c ClusterOutputClient) List(_ string, _ metav1.ListOptions) (*loggingv1.ClusterOutputList, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c ClusterOutputClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*loggingv1.ClusterOutput, *loggingv1.ClusterOutputList], error) {
	panic("implement me")
}

type ClusterOutputCache func() loggingv1type.ClusterOutputInterface

func (c ClusterOutputCache) Get(_, _ string) (*loggingv1.ClusterOutput, error) {
	panic("implement me")
}
func (c ClusterOutputCache) List(namespace string, selector labels.Selector) ([]*loggingv1.ClusterOutput, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*loggingv1.ClusterOutput, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Namespace == namespace {
			result = append(result, &list.Items[i])
		}
	}
	return result, err
}
func (c ClusterOutputCache) AddIndexer(_ string, _ generic.Indexer[*loggingv1.ClusterOutput]) {
	panic("implement me")
}
func (c ClusterOutputCache) GetByIndex(_, _ string) ([]*loggingv1.ClusterOutput, error) {
	panic("implement me")
}

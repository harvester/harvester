package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type NodeCache func() corev1type.NodeInterface

func (c NodeCache) Get(name string) (*v1.Node, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c NodeCache) List(selector labels.Selector) ([]*v1.Node, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*v1.Node, 0, len(list.Items))
	for _, node := range list.Items {
		obj := node
		result = append(result, &obj)
	}
	return result, err
}

func (c NodeCache) AddIndexer(_ string, _ generic.Indexer[*v1.Node]) {
	panic("implement me")
}

func (c NodeCache) GetByIndex(_, _ string) ([]*v1.Node, error) {
	panic("implement me")
}

type NodeClient func() corev1type.NodeInterface

func (c NodeClient) Update(node *v1.Node) (*v1.Node, error) {
	return c().Update(context.TODO(), node, metav1.UpdateOptions{})
}

func (c NodeClient) Get(name string, options metav1.GetOptions) (*v1.Node, error) {
	return c().Get(context.TODO(), name, options)
}

func (c NodeClient) Create(node *v1.Node) (*v1.Node, error) {
	return c().Create(context.TODO(), node, metav1.CreateOptions{})
}

func (c NodeClient) Delete(_ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c NodeClient) List(metav1.ListOptions) (*v1.NodeList, error) {
	panic("implement me")
}

func (c NodeClient) UpdateStatus(*v1.Node) (*v1.Node, error) {
	panic("implement me")
}

func (c NodeClient) Watch(metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c NodeClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (result *v1.Node, err error) {
	panic("implement me")
}

func (c NodeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*v1.Node, *v1.NodeList], error) {
	panic("implement me")
}

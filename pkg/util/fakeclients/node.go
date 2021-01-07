package fakeclients

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
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
		result = append(result, &node)
	}
	return result, err
}
func (c NodeCache) AddIndexer(indexName string, indexer ctlcorev1.NodeIndexer) {
	panic("implement me")
}
func (c NodeCache) GetByIndex(indexName, key string) ([]*v1.Node, error) {
	panic("implement me")
}

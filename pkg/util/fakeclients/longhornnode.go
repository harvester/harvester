package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"

	lhtype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type LonghornNodeCache func(string) lhtype.NodeInterface

func (c LonghornNodeCache) Get(namespace, name string) (*lhv1beta2.Node, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornNodeCache) List(namespace string, selector labels.Selector) ([]*lhv1beta2.Node, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*lhv1beta2.Node, 0, len(list.Items))
	for _, item := range list.Items {
		obj := item
		result = append(result, &obj)
	}
	return result, err
}

func (c LonghornNodeCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.Node]) {
	panic("implement me")
}

func (c LonghornNodeCache) GetByIndex(_, _ string) ([]*lhv1beta2.Node, error) {
	panic("implement me")
}

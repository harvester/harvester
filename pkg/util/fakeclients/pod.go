package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type PodCache func(namespace string) v1.PodInterface

func (c PodCache) Get(namespace, name string) (*corev1.Pod, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c PodCache) List(namespace string, selector labels.Selector) ([]*corev1.Pod, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*corev1.Pod, 0, len(list.Items))
	for _, item := range list.Items {
		obj := item
		result = append(result, &obj)
	}
	return result, err
}

func (c PodCache) AddIndexer(_ string, _ generic.Indexer[*corev1.Pod]) {
	panic("implement me")
}

func (c PodCache) GetByIndex(_, _ string) ([]*corev1.Pod, error) {
	panic("implement me")
}

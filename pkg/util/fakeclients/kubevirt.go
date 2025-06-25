package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1api "kubevirt.io/api/core/v1"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

type KubeVirtCache func(string) kubevirtv1.KubeVirtInterface

func (c KubeVirtCache) Get(namespace, name string) (*kubevirtv1api.KubeVirt, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c KubeVirtCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.KubeVirt, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1api.KubeVirt, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c KubeVirtCache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1api.KubeVirt]) {
	panic("implement me")
}

func (c KubeVirtCache) GetByIndex(_, _ string) ([]*kubevirtv1api.KubeVirt, error) {
	panic("implement me")
}

package fakeclients

import (
	"context"

	kubeovnapiv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	kubeovnv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubeovn.io/v1"
)

type KubeovnSubnetCache func() kubeovnv1.SubnetInterface

func (c KubeovnSubnetCache) Get(namespace, name string) (*kubeovnapiv1.Subnet, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c KubeovnSubnetCache) List(namespace string, selector labels.Selector) ([]*kubeovnapiv1.Subnet, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubeovnapiv1.Subnet, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c KubeovnSubnetCache) AddIndexer(_ string, _ generic.Indexer[*kubeovnapiv1.Subnet]) {
	panic("implement me")
}

func (c KubeovnSubnetCache) GetByIndex(_, _ string) ([]*kubeovnapiv1.Subnet, error) {
	panic("implement me")
}

package fakeclients

import (
	"context"

	"github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	whereaboutstype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/whereabouts.cni.cncf.io/v1alpha1"
)

// WhereaboutsIPPoolCache ignores namespaces because the generated IPPool client is not namespaced.
type WhereaboutsIPPoolCache func() whereaboutstype.IPPoolInterface

func (c WhereaboutsIPPoolCache) Get(_, name string) (*v1alpha1.IPPool, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c WhereaboutsIPPoolCache) List(_ string, selector labels.Selector) ([]*v1alpha1.IPPool, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1alpha1.IPPool, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c WhereaboutsIPPoolCache) AddIndexer(_ string, _ generic.Indexer[*v1alpha1.IPPool]) {
	panic("implement me")
}

func (c WhereaboutsIPPoolCache) GetByIndex(_, _ string) ([]*v1alpha1.IPPool, error) {
	panic("implement me")
}

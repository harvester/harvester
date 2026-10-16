package fakeclients

import (
	"context"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	whereaboutstype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/whereabouts.cni.cncf.io/v1alpha1"
)

type WhereaboutsIPPoolClient func() whereaboutstype.IPPoolInterface

func (c WhereaboutsIPPoolClient) Create(pool *whereaboutsv1alpha1.IPPool) (*whereaboutsv1alpha1.IPPool, error) {
	poolCopy := pool.DeepCopy()
	poolCopy.Namespace = ""
	return c().Create(context.TODO(), poolCopy, metav1.CreateOptions{})
}

func (c WhereaboutsIPPoolClient) Update(pool *whereaboutsv1alpha1.IPPool) (*whereaboutsv1alpha1.IPPool, error) {
	poolCopy := pool.DeepCopy()
	poolCopy.Namespace = ""
	return c().Update(context.TODO(), poolCopy, metav1.UpdateOptions{})
}

func (c WhereaboutsIPPoolClient) UpdateStatus(*whereaboutsv1alpha1.IPPool) (*whereaboutsv1alpha1.IPPool, error) {
	panic("implement me")
}

func (c WhereaboutsIPPoolClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c WhereaboutsIPPoolClient) Get(namespace, name string, options metav1.GetOptions) (*whereaboutsv1alpha1.IPPool, error) {
	return c().Get(context.TODO(), name, options)
}

func (c WhereaboutsIPPoolClient) List(namespace string, opts metav1.ListOptions) (*whereaboutsv1alpha1.IPPoolList, error) {
	return c().List(context.TODO(), opts)
}

func (c WhereaboutsIPPoolClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c WhereaboutsIPPoolClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *whereaboutsv1alpha1.IPPool, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c WhereaboutsIPPoolClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*whereaboutsv1alpha1.IPPool, *whereaboutsv1alpha1.IPPoolList], error) {
	panic("implement me")
}

type WhereaboutsIPPoolCache func() whereaboutstype.IPPoolInterface

func (c WhereaboutsIPPoolCache) Get(namespace, name string) (*whereaboutsv1alpha1.IPPool, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c WhereaboutsIPPoolCache) List(namespace string, selector labels.Selector) ([]*whereaboutsv1alpha1.IPPool, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*whereaboutsv1alpha1.IPPool, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c WhereaboutsIPPoolCache) AddIndexer(_ string, _ generic.Indexer[*whereaboutsv1alpha1.IPPool]) {
	panic("implement me")
}

func (c WhereaboutsIPPoolCache) GetByIndex(_, _ string) ([]*whereaboutsv1alpha1.IPPool, error) {
	panic("implement me")
}

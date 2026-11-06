package fakeclients

import (
	"context"

	discoverytype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/discovery.k8s.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type EndpointSliceClient func(string) discoverytype.EndpointSliceInterface

func (c EndpointSliceClient) Create(endpointSlice *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	return c(endpointSlice.Namespace).Create(context.TODO(), endpointSlice, metav1.CreateOptions{})
}

func (c EndpointSliceClient) Update(endpointSlice *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	return c(endpointSlice.Namespace).Update(context.TODO(), endpointSlice, metav1.UpdateOptions{})
}

func (c EndpointSliceClient) UpdateStatus(*discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, error) {
	panic("implement me")
}

func (c EndpointSliceClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c EndpointSliceClient) Get(namespace, name string, options metav1.GetOptions) (*discoveryv1.EndpointSlice, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c EndpointSliceClient) List(namespace string, opts metav1.ListOptions) (*discoveryv1.EndpointSliceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c EndpointSliceClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c EndpointSliceClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *discoveryv1.EndpointSlice, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
func (c EndpointSliceClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*discoveryv1.EndpointSlice, *discoveryv1.EndpointSliceList], error) {
	panic("implement me")
}

type EndpointSliceCache func(string) discoverytype.EndpointSliceInterface

func (c EndpointSliceCache) Get(namespace, name string) (*discoveryv1.EndpointSlice, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c EndpointSliceCache) List(_ string, _ labels.Selector) ([]*discoveryv1.EndpointSlice, error) {
	panic("implement me")
}

func (c EndpointSliceCache) AddIndexer(_ string, _ generic.Indexer[*discoveryv1.EndpointSlice]) {
	panic("implement me")
}

func (c EndpointSliceCache) GetByIndex(_, _ string) ([]*discoveryv1.EndpointSlice, error) {
	panic("implement me")
}

package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type ServiceClient func(string) v1.ServiceInterface

func (c ServiceClient) Create(service *corev1.Service) (*corev1.Service, error) {
	return c(service.Namespace).Create(context.TODO(), service, metav1.CreateOptions{})
}

func (c ServiceClient) Update(service *corev1.Service) (*corev1.Service, error) {
	return c(service.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
}

func (c ServiceClient) UpdateStatus(*corev1.Service) (*corev1.Service, error) {
	panic("implement me")
}

func (c ServiceClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c ServiceClient) Get(namespace, name string, options metav1.GetOptions) (*corev1.Service, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c ServiceClient) List(namespace string, opts metav1.ListOptions) (*corev1.ServiceList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c ServiceClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c ServiceClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *corev1.Service, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
func (c ServiceClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*corev1.Service, *corev1.ServiceList], error) {
	panic("implement me")
}

type ServiceCache func(string) v1.ServiceInterface

func (c ServiceCache) Get(namespace, name string) (*corev1.Service, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ServiceCache) List(_ string, _ labels.Selector) ([]*corev1.Service, error) {
	panic("implement me")
}

func (c ServiceCache) AddIndexer(_ string, _ generic.Indexer[*corev1.Service]) {
	panic("implement me")
}

func (c ServiceCache) GetByIndex(_, _ string) ([]*corev1.Service, error) {
	panic("implement me")
}

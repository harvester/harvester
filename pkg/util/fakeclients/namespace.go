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
)

type NamespaceCache func() corev1type.NamespaceInterface

func (c NamespaceCache) Get(name string) (*v1.Namespace, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c NamespaceCache) List(selector labels.Selector) ([]*v1.Namespace, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*v1.Namespace, 0, len(list.Items))
	for _, namespace := range list.Items {
		obj := namespace
		result = append(result, &obj)
	}
	return result, err
}

func (c NamespaceCache) AddIndexer(_ string, _ generic.Indexer[*v1.Namespace]) {
	panic("implement me")
}

func (c NamespaceCache) GetByIndex(_, _ string) ([]*v1.Namespace, error) {
	panic("implement me")
}

type NamespaceClient func() corev1type.NamespaceInterface

func (c NamespaceClient) Update(namespace *v1.Namespace) (*v1.Namespace, error) {
	return c().Update(context.TODO(), namespace, metav1.UpdateOptions{})
}

func (c NamespaceClient) Get(name string, options metav1.GetOptions) (*v1.Namespace, error) {
	return c().Get(context.TODO(), name, options)
}

func (c NamespaceClient) Create(namespace *v1.Namespace) (*v1.Namespace, error) {
	return c().Create(context.TODO(), namespace, metav1.CreateOptions{})
}

func (c NamespaceClient) Delete(_ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c NamespaceClient) List(metav1.ListOptions) (*v1.NamespaceList, error) {
	panic("implement me")
}

func (c NamespaceClient) UpdateStatus(*v1.Namespace) (*v1.Namespace, error) {
	panic("implement me")
}

func (c NamespaceClient) Watch(metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c NamespaceClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (result *v1.Namespace, err error) {
	panic("implement me")
}

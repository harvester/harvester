package fakeclients

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
)

type ConfigmapClient func(namespace string) corev1type.ConfigMapInterface

func (c ConfigmapClient) Create(configMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	return c(configMap.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
}

func (c ConfigmapClient) Update(configMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	return c(configMap.Namespace).Update(context.TODO(), configMap, metav1.UpdateOptions{})
}

func (c ConfigmapClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c ConfigmapClient) Get(namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c ConfigmapClient) List(namespace string, opts metav1.ListOptions) (*v1.ConfigMapList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c ConfigmapClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c ConfigmapClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ConfigMap, err error) {
	panic("implement me")
}

type ConfigmapCache func(namespace string) corev1type.ConfigMapInterface

func (c ConfigmapCache) Get(namespace, name string) (*v1.ConfigMap, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ConfigmapCache) List(namespace string, selector labels.Selector) ([]*v1.ConfigMap, error) {
	panic("implement me")
}

func (c ConfigmapCache) AddIndexer(indexName string, indexer ctlcorev1.ConfigMapIndexer) {
	panic("implement me")
}

func (c ConfigmapCache) GetByIndex(indexName, key string) ([]*v1.ConfigMap, error) {
	panic("implement me")
}

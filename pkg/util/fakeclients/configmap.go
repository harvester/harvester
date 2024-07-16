package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"

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

func (c ConfigmapClient) UpdateStatus(_ *v1.ConfigMap) (*v1.ConfigMap, error) {
	//TODO implement me
	panic("implement me")
}

func (c ConfigmapClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c ConfigmapClient) Get(namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c ConfigmapClient) List(namespace string, opts metav1.ListOptions) (*v1.ConfigMapList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c ConfigmapClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c ConfigmapClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *v1.ConfigMap, err error) {
	panic("implement me")
}

func (c ConfigmapClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*v1.ConfigMap, *v1.ConfigMapList], error) {
	panic("implement me")
}

type ConfigmapCache func(namespace string) corev1type.ConfigMapInterface

func (c ConfigmapCache) Get(namespace, name string) (*v1.ConfigMap, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ConfigmapCache) List(namespace string, selector labels.Selector) ([]*v1.ConfigMap, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*v1.ConfigMap, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c ConfigmapCache) AddIndexer(_ string, _ generic.Indexer[*v1.ConfigMap]) {
	panic("implement me")
}

func (c ConfigmapCache) GetByIndex(_, _ string) ([]*v1.ConfigMap, error) {
	panic("implement me")
}

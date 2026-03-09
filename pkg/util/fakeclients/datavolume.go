package fakeclients

import (
	"context"

	cditype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

type DataVolumeClient func() cditype.DataVolumeInterface

func (c DataVolumeClient) Create(dataVolume *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	return c().Create(context.TODO(), dataVolume, metav1.CreateOptions{})
}

func (c DataVolumeClient) Update(dataVolume *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	return c().Update(context.TODO(), dataVolume, metav1.UpdateOptions{})
}

func (c DataVolumeClient) UpdateStatus(*cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("implement me")
}

func (c DataVolumeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c DataVolumeClient) Get(namespace, name string, options metav1.GetOptions) (*cdiv1.DataVolume, error) {
	return c().Get(context.TODO(), name, options)
}

func (c DataVolumeClient) List(namespace string, opts metav1.ListOptions) (*cdiv1.DataVolumeList, error) {
	return c().List(context.TODO(), opts)
}

func (c DataVolumeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c DataVolumeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *cdiv1.DataVolume, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c DataVolumeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*cdiv1.DataVolume, *cdiv1.DataVolumeList], error) {
	panic("implement me")
}

type DataVolumeCache func(string) cditype.DataVolumeInterface

func (c DataVolumeCache) Get(namespace, name string) (*cdiv1.DataVolume, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c DataVolumeCache) List(namespace string, selector labels.Selector) ([]*cdiv1.DataVolume, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*cdiv1.DataVolume, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c DataVolumeCache) AddIndexer(indexName string, indexer generic.Indexer[*cdiv1.DataVolume]) {
	panic("implement me")
}

func (c DataVolumeCache) GetByIndex(indexName, key string) ([]*cdiv1.DataVolume, error) {
	panic("implement me")
}

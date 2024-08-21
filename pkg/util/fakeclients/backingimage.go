package fakeclients

import (
	"context"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	longhornv1beta2 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
)

type BackingImageClient func(string) longhornv1beta2.BackingImageInterface

func (c BackingImageClient) Create(bi *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	return c(bi.Namespace).Create(context.TODO(), bi, metav1.CreateOptions{})
}

func (c BackingImageClient) Update(bi *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	return c(bi.Namespace).Update(context.TODO(), bi, metav1.UpdateOptions{})
}

func (c BackingImageClient) UpdateStatus(bi *longhorn.BackingImage) (*longhorn.BackingImage, error) {
	return c(bi.Namespace).UpdateStatus(context.TODO(), bi, metav1.UpdateOptions{})
}

func (c BackingImageClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c BackingImageClient) Get(namespace, name string, options metav1.GetOptions) (*longhorn.BackingImage, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c BackingImageClient) List(namespace string, opts metav1.ListOptions) (*longhorn.BackingImageList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c BackingImageClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c BackingImageClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *longhorn.BackingImage, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type BackingImageCache func(string) longhornv1beta2.BackingImageInterface

func (c BackingImageCache) Get(namespace, name string) (*longhorn.BackingImage, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c BackingImageCache) List(namespace string, selector labels.Selector) ([]*longhorn.BackingImage, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*longhorn.BackingImage, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c BackingImageClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*longhorn.BackingImage, *longhorn.BackingImageList], error) {
	//TODO implement me
	panic("implement me")
}

func (c BackingImageCache) AddIndexer(_ string, _ generic.Indexer[*longhorn.BackingImage]) {
	panic("implement me")
}

func (c BackingImageCache) GetByIndex(_, _ string) ([]*longhorn.BackingImage, error) {
	panic("implement me")
}

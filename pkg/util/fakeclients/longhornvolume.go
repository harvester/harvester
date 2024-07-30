package fakeclients

import (
	"context"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	longhornv1beta2 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
)

type LonghornVolumeClient func(string) longhornv1beta2.VolumeInterface

func (c LonghornVolumeClient) Create(volume *lhv1beta2.Volume) (*lhv1beta2.Volume, error) {
	return c(volume.Namespace).Create(context.TODO(), volume, metav1.CreateOptions{})
}

func (c LonghornVolumeClient) Update(volume *lhv1beta2.Volume) (*lhv1beta2.Volume, error) {
	return c(volume.Namespace).Update(context.TODO(), volume, metav1.UpdateOptions{})
}

func (c LonghornVolumeClient) UpdateStatus(*lhv1beta2.Volume) (*lhv1beta2.Volume, error) {
	panic("implement me")
}

func (c LonghornVolumeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornVolumeClient) Get(namespace, name string, options metav1.GetOptions) (*lhv1beta2.Volume, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornVolumeClient) List(namespace string, opts metav1.ListOptions) (*lhv1beta2.VolumeList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornVolumeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornVolumeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *lhv1beta2.Volume, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type LonghornVolumeCache func(string) longhornv1beta2.VolumeInterface

func (c LonghornVolumeCache) Get(namespace, name string) (*lhv1beta2.Volume, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornVolumeCache) List(namespace string, selector labels.Selector) ([]*lhv1beta2.Volume, error) {
	volumeList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	returnVolumes := make([]*lhv1beta2.Volume, 0, len(volumeList.Items))
	for i := range volumeList.Items {
		returnVolumes = append(returnVolumes, &volumeList.Items[i])
	}

	return returnVolumes, nil
}

func (c LonghornVolumeCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.Volume]) {
	panic("implement me")
}

func (c LonghornVolumeCache) GetByIndex(_, _ string) ([]*lhv1beta2.Volume, error) {
	panic("implement me")
}

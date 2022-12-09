package fakeclients

import (
	"context"

	longhornv1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	lhtype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta1"
	longhornv1ctl "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
)

type LonghornVolumeClient func(string) lhtype.VolumeInterface

func (c LonghornVolumeClient) Create(volume *longhornv1.Volume) (*longhornv1.Volume, error) {
	return c(volume.Namespace).Create(context.TODO(), volume, metav1.CreateOptions{})
}

func (c LonghornVolumeClient) Update(volume *longhornv1.Volume) (*longhornv1.Volume, error) {
	return c(volume.Namespace).Update(context.TODO(), volume, metav1.UpdateOptions{})
}

func (c LonghornVolumeClient) UpdateStatus(setting *longhornv1.Setting) (*longhornv1.Volume, error) {
	panic("implement me")
}

func (c LonghornVolumeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornVolumeClient) Get(namespace, name string, options metav1.GetOptions) (*longhornv1.Volume, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornVolumeClient) List(namespace string, opts metav1.ListOptions) (*longhornv1.VolumeList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornVolumeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornVolumeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *longhornv1.Volume, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type LonghornVolumeCache func(string) lhtype.VolumeInterface

func (c LonghornVolumeCache) Get(namespace, name string) (*longhornv1.Volume, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornVolumeCache) List(namespace string, selector labels.Selector) ([]*longhornv1.Volume, error) {
	volumeList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	returnVolumes := make([]*longhornv1.Volume, 0, len(volumeList.Items))
	for i := range volumeList.Items {
		returnVolumes = append(returnVolumes, &volumeList.Items[i])
	}

	return returnVolumes, nil
}

func (c LonghornVolumeCache) AddIndexer(indexName string, indexer longhornv1ctl.VolumeIndexer) {
	panic("implement me")
}

func (c LonghornVolumeCache) GetByIndex(indexName, key string) ([]*longhornv1.Volume, error) {
	panic("implement me")
}

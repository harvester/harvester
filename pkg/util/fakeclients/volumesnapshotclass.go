package fakeclients

import (
	"context"

	snapshotv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/snapshot.storage.k8s.io/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type VolumeSnapshotClassClient func() snapshotv1type.VolumeSnapshotClassInterface

func (c VolumeSnapshotClassClient) Create(obj *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshotClass, error) {
	return c().Create(context.TODO(), obj, metav1.CreateOptions{})
}

func (c VolumeSnapshotClassClient) Update(obj *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshotClass, error) {
	return c().Update(context.TODO(), obj, metav1.UpdateOptions{})
}

func (c VolumeSnapshotClassClient) UpdateStatus(_ *snapshotv1.VolumeSnapshotClass) (*snapshotv1.VolumeSnapshotClass, error) {
	panic("implement me")
}

func (c VolumeSnapshotClassClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c VolumeSnapshotClassClient) Get(name string, options metav1.GetOptions) (*snapshotv1.VolumeSnapshotClass, error) {
	return c().Get(context.TODO(), name, options)
}

func (c VolumeSnapshotClassClient) List(opts metav1.ListOptions) (*snapshotv1.VolumeSnapshotClassList, error) {
	return c().List(context.TODO(), opts)
}

func (c VolumeSnapshotClassClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c VolumeSnapshotClassClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *snapshotv1.VolumeSnapshotClass, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type VolumeSnapshotClassCache func() snapshotv1type.VolumeSnapshotClassInterface

func (c VolumeSnapshotClassCache) Get(name string) (*snapshotv1.VolumeSnapshotClass, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VolumeSnapshotClassCache) List(selector labels.Selector) ([]*snapshotv1.VolumeSnapshotClass, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*snapshotv1.VolumeSnapshotClass, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c VolumeSnapshotClassClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*snapshotv1.VolumeSnapshotClass, *snapshotv1.VolumeSnapshotClassList], error) {
	panic("implement me")
}

func (c VolumeSnapshotClassCache) AddIndexer(_ string, _ generic.Indexer[*snapshotv1.VolumeSnapshotClass]) {
	panic("implement me")
}

func (c VolumeSnapshotClassCache) GetByIndex(indexName, key string) ([]*snapshotv1.VolumeSnapshotClass, error) {
	panic("implement me")
}

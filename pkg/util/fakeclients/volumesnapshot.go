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

type VolumeSnapshotClient func(string) snapshotv1type.VolumeSnapshotInterface

func (c VolumeSnapshotClient) Create(vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	return c(vs.Namespace).Create(context.TODO(), vs, metav1.CreateOptions{})
}

func (c VolumeSnapshotClient) Update(vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	return c(vs.Namespace).Update(context.TODO(), vs, metav1.UpdateOptions{})
}

func (c VolumeSnapshotClient) UpdateStatus(vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	return c(vs.Namespace).UpdateStatus(context.TODO(), vs, metav1.UpdateOptions{})
}

func (c VolumeSnapshotClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c VolumeSnapshotClient) Get(
	namespace, name string,
	options metav1.GetOptions,
) (*snapshotv1.VolumeSnapshot, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VolumeSnapshotClient) List(
	namespace string,
	opts metav1.ListOptions,
) (*snapshotv1.VolumeSnapshotList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c VolumeSnapshotClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c VolumeSnapshotClient) Patch(
	namespace, name string,
	pt types.PatchType,
	data []byte,
	subresources ...string,
) (*snapshotv1.VolumeSnapshot, error) {
	return c(namespace).Patch(
		context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...,
	)
}

func (c VolumeSnapshotClient) WithImpersonation(
	_ rest.ImpersonationConfig,
) (generic.ClientInterface[*snapshotv1.VolumeSnapshot, *snapshotv1.VolumeSnapshotList], error) {
	panic("implement me")
}

type VolumeSnapshotCache func(string) snapshotv1type.VolumeSnapshotInterface

func (c VolumeSnapshotCache) Get(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VolumeSnapshotCache) List(namespace string, selector labels.Selector) ([]*snapshotv1.VolumeSnapshot, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*snapshotv1.VolumeSnapshot, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c VolumeSnapshotCache) AddIndexer(_ string, _ generic.Indexer[*snapshotv1.VolumeSnapshot]) {
	panic("implement me")
}

func (c VolumeSnapshotCache) GetByIndex(_, _ string) ([]*snapshotv1.VolumeSnapshot, error) {
	panic("implement me")
}

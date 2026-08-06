package fakeclients

import (
	"context"

	snapshotv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/snapshot.storage.k8s.io/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

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

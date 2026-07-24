package fakeclients

import (
	"context"

	snapshotv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/snapshot.storage.k8s.io/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type VolumeSnapshotContentCache func() snapshotv1type.VolumeSnapshotContentInterface

func (c VolumeSnapshotContentCache) Get(name string) (*snapshotv1.VolumeSnapshotContent, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VolumeSnapshotContentCache) List(selector labels.Selector) ([]*snapshotv1.VolumeSnapshotContent, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*snapshotv1.VolumeSnapshotContent, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c VolumeSnapshotContentCache) AddIndexer(_ string, _ generic.Indexer[*snapshotv1.VolumeSnapshotContent]) {
	panic("implement me")
}

func (c VolumeSnapshotContentCache) GetByIndex(_, _ string) ([]*snapshotv1.VolumeSnapshotContent, error) {
	panic("implement me")
}

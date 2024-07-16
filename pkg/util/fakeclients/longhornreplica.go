package fakeclients

import (
	"context"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	lhtype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
)

type LonghornReplicaClient func(string) lhtype.ReplicaInterface

func (c LonghornReplicaClient) Create(replica *lhv1beta2.Replica) (*lhv1beta2.Replica, error) {
	return c(replica.Namespace).Create(context.TODO(), replica, metav1.CreateOptions{})
}

func (c LonghornReplicaClient) Update(replica *lhv1beta2.Replica) (*lhv1beta2.Replica, error) {
	return c(replica.Namespace).Update(context.TODO(), replica, metav1.UpdateOptions{})
}

func (c LonghornReplicaClient) UpdateStatus(_ *lhv1beta2.Replica) (*lhv1beta2.Replica, error) {
	panic("implement me")
}

func (c LonghornReplicaClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornReplicaClient) Get(namespace, name string, options metav1.GetOptions) (*lhv1beta2.Replica, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornReplicaClient) List(namespace string, opts metav1.ListOptions) (*lhv1beta2.ReplicaList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornReplicaClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornReplicaClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *lhv1beta2.Replica, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type LonghornReplicaCache func(string) lhtype.ReplicaInterface

func (c LonghornReplicaCache) Get(namespace, name string) (*lhv1beta2.Replica, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornReplicaCache) List(namespace string, selector labels.Selector) ([]*lhv1beta2.Replica, error) {
	replicaList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	returnReplicas := make([]*lhv1beta2.Replica, 0, len(replicaList.Items))
	for i := range replicaList.Items {
		returnReplicas = append(returnReplicas, &replicaList.Items[i])
	}

	return returnReplicas, nil
}

func (c LonghornReplicaCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.Replica]) {
	panic("implement me")
}

func (c LonghornReplicaCache) GetByIndex(_, _ string) ([]*lhv1beta2.Replica, error) {
	panic("implement me")
}

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

type LonghornReplicaClient func(string) lhtype.ReplicaInterface

func (c LonghornReplicaClient) Create(replica *longhornv1.Replica) (*longhornv1.Replica, error) {
	return c(replica.Namespace).Create(context.TODO(), replica, metav1.CreateOptions{})
}

func (c LonghornReplicaClient) Update(replica *longhornv1.Replica) (*longhornv1.Replica, error) {
	return c(replica.Namespace).Update(context.TODO(), replica, metav1.UpdateOptions{})
}

func (c LonghornReplicaClient) UpdateStatus(replica *longhornv1.Replica) (*longhornv1.Replica, error) {
	panic("implement me")
}

func (c LonghornReplicaClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornReplicaClient) Get(namespace, name string, options metav1.GetOptions) (*longhornv1.Replica, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornReplicaClient) List(namespace string, opts metav1.ListOptions) (*longhornv1.ReplicaList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornReplicaClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornReplicaClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *longhornv1.Replica, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type LonghornReplicaCache func(string) lhtype.ReplicaInterface

func (c LonghornReplicaCache) Get(namespace, name string) (*longhornv1.Replica, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornReplicaCache) List(namespace string, selector labels.Selector) ([]*longhornv1.Replica, error) {
	replicaList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	returnReplicas := make([]*longhornv1.Replica, 0, len(replicaList.Items))
	for i := range replicaList.Items {
		returnReplicas = append(returnReplicas, &replicaList.Items[i])
	}

	return returnReplicas, nil
}

func (c LonghornReplicaCache) AddIndexer(indexName string, indexer longhornv1ctl.ReplicaIndexer) {
	panic("implement me")
}

func (c LonghornReplicaCache) GetByIndex(indexName, key string) ([]*longhornv1.Replica, error) {
	panic("implement me")
}

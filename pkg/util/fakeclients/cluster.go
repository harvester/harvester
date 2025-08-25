package fakeclients

import (
	"context"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	provisioningv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/provisioning.cattle.io/v1"
)

type ClusterClient func(string) provisioningv1type.ClusterInterface

func (c ClusterClient) Create(cluster *provisioningv1.Cluster) (*provisioningv1.Cluster, error) {
	return c(cluster.Namespace).Create(context.TODO(), cluster, metav1.CreateOptions{})
}
func (c ClusterClient) Update(cluster *provisioningv1.Cluster) (*provisioningv1.Cluster, error) {
	return c(cluster.Namespace).Update(context.TODO(), cluster, metav1.UpdateOptions{})
}
func (c ClusterClient) UpdateStatus(cluster *provisioningv1.Cluster) (*provisioningv1.Cluster, error) {
	panic("implement me")
}
func (c ClusterClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c ClusterClient) Get(namespace, name string, options metav1.GetOptions) (*provisioningv1.Cluster, error) {
	return c(namespace).Get(context.TODO(), name, options)
}
func (c ClusterClient) List(namespace string, opts metav1.ListOptions) (*provisioningv1.ClusterList, error) {
	panic("implement me")
}
func (c ClusterClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c ClusterClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *provisioningv1.Cluster, err error) {
	panic("implement me")
}
func (c ClusterClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*provisioningv1.Cluster, *provisioningv1.ClusterList], error) {
	panic("implement me")
}

type ClusterCache func(string) provisioningv1type.ClusterInterface

func (c ClusterCache) Get(namespace, name string) (*provisioningv1.Cluster, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ClusterCache) List(namespace string, selector labels.Selector) ([]*provisioningv1.Cluster, error) {
	panic("implement me")
}
func (c ClusterCache) AddIndexer(indexName string, indexer generic.Indexer[*provisioningv1.Cluster]) {
	panic("implement me")
}
func (c ClusterCache) GetByIndex(indexName, key string) ([]*provisioningv1.Cluster, error) {
	panic("implement me")
}

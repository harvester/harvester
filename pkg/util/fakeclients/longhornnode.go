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

type LonghornNodeClient func(string) longhornv1beta2.NodeInterface

func (c LonghornNodeClient) Create(node *lhv1beta2.Node) (*lhv1beta2.Node, error) {
	return c(node.Namespace).Create(context.TODO(), node, metav1.CreateOptions{})
}

func (c LonghornNodeClient) Update(node *lhv1beta2.Node) (*lhv1beta2.Node, error) {
	return c(node.Namespace).Update(context.TODO(), node, metav1.UpdateOptions{})
}

func (c LonghornNodeClient) UpdateStatus(*lhv1beta2.Node) (*lhv1beta2.Node, error) {
	panic("implement me")
}

func (c LonghornNodeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornNodeClient) Get(namespace, name string, options metav1.GetOptions) (*lhv1beta2.Node, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornNodeClient) List(namespace string, opts metav1.ListOptions) (*lhv1beta2.NodeList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornNodeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornNodeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *lhv1beta2.Node, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type LonghornNodeCache func(string) longhornv1beta2.NodeInterface

func (c LonghornNodeCache) Get(namespace, name string) (*lhv1beta2.Node, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornNodeCache) List(namespace string, selector labels.Selector) ([]*lhv1beta2.Node, error) {
	nodeList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	returnNodes := make([]*lhv1beta2.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		returnNodes = append(returnNodes, &nodeList.Items[i])
	}

	return returnNodes, nil
}

func (c LonghornNodeCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.Node]) {
	panic("implement me")
}

func (c LonghornNodeCache) GetByIndex(_, _ string) ([]*lhv1beta2.Node, error) {
	panic("implement me")
}

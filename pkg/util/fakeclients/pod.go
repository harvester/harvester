package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type PodClient func(string) v1.PodInterface

func (c PodClient) Create(pod *corev1.Pod) (*corev1.Pod, error) {
	return c(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
}

func (c PodClient) Update(pod *corev1.Pod) (*corev1.Pod, error) {
	return c(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
}

func (c PodClient) UpdateStatus(*corev1.Pod) (*corev1.Pod, error) {
	panic("implement me")
}

func (c PodClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c PodClient) Get(namespace, name string, options metav1.GetOptions) (*corev1.Pod, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c PodClient) List(namespace string, opts metav1.ListOptions) (*corev1.PodList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c PodClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c PodClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *corev1.Pod, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c PodClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*corev1.Pod, *corev1.PodList], error) {
	panic("implement me")
}

type PodCache func(string) v1.PodInterface

func (c PodCache) Get(namespace, name string) (*corev1.Pod, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c PodCache) List(namespace string, selector labels.Selector) ([]*corev1.Pod, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}

func (c PodCache) AddIndexer(_ string, _ generic.Indexer[*corev1.Pod]) {
	panic("implement me")
}

func (c PodCache) GetByIndex(indexName, key string) ([]*corev1.Pod, error) {
	panic("implement me for index: " + indexName)
}

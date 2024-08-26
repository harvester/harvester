package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type SecretClient func(string) corev1type.SecretInterface

func (c SecretClient) Update(secret *corev1.Secret) (*corev1.Secret, error) {
	return c(secret.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
}

func (c SecretClient) Get(namespace, name string, options metav1.GetOptions) (*corev1.Secret, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c SecretClient) Create(secret *corev1.Secret) (*corev1.Secret, error) {
	return c(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
}

func (c SecretClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c SecretClient) List(namespace string, options metav1.ListOptions) (*corev1.SecretList, error) {
	return c(namespace).List(context.TODO(), options)
}

func (c SecretClient) UpdateStatus(_ *corev1.Secret) (*corev1.Secret, error) {
	panic("implement me")
}

func (c SecretClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c SecretClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*corev1.Secret, error) {
	panic("implement me")
}

func (c SecretClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*corev1.Secret, *corev1.SecretList], error) {
	panic("implement me")
}

type SecretCache func(string) corev1type.SecretInterface

func (c SecretCache) Get(namespace, name string) (*corev1.Secret, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c SecretCache) List(namespace string, selector labels.Selector) ([]*corev1.Secret, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*corev1.Secret, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c SecretCache) AddIndexer(_ string, _ generic.Indexer[*corev1.Secret]) {
	panic("implement me")
}

func (c SecretCache) GetByIndex(_, _ string) ([]*corev1.Secret, error) {
	panic("implement me")
}

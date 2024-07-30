package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type ResourceQuotaCache func(string) corev1type.ResourceQuotaInterface

func (c ResourceQuotaCache) Get(namespace, name string) (*v1.ResourceQuota, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c ResourceQuotaCache) List(namespace string, selector labels.Selector) ([]*v1.ResourceQuota, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*v1.ResourceQuota, 0, len(list.Items))
	for _, quota := range list.Items {
		obj := quota
		result = append(result, &obj)
	}
	return result, err
}

func (c ResourceQuotaCache) AddIndexer(_ string, _ generic.Indexer[*v1.ResourceQuota]) {
	panic("implement me")
}

func (c ResourceQuotaCache) GetByIndex(_, _ string) ([]*v1.ResourceQuota, error) {
	panic("implement me")
}

type ResourceQuotaClient func(string) corev1type.ResourceQuotaInterface

func (c ResourceQuotaClient) Update(quota *v1.ResourceQuota) (*v1.ResourceQuota, error) {
	return c(quota.Namespace).Update(context.TODO(), quota, metav1.UpdateOptions{})
}

func (c ResourceQuotaClient) Get(namespace string, name string, options metav1.GetOptions) (*v1.ResourceQuota, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c ResourceQuotaClient) Create(quota *v1.ResourceQuota) (*v1.ResourceQuota, error) {
	return c(quota.Namespace).Create(context.TODO(), quota, metav1.CreateOptions{})
}

func (c ResourceQuotaClient) Delete(_ string, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c ResourceQuotaClient) List(_ string, _ metav1.ListOptions) (*v1.ResourceQuotaList, error) {
	panic("implement me")
}

func (c ResourceQuotaClient) UpdateStatus(*v1.ResourceQuota) (*v1.ResourceQuota, error) {
	panic("implement me")
}

func (c ResourceQuotaClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c ResourceQuotaClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *v1.ResourceQuota, err error) {
	panic("implement me")
}

func (c ResourceQuotaClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*v1.ResourceQuota, *v1.ResourceQuotaList], error) {
	panic("implement me")
}

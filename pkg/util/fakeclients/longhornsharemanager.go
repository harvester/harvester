package fakeclients

import (
	"context"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	longhornv1beta2 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/longhorn.io/v1beta2"
)

type LonghornShareManagerClient func(string) longhornv1beta2.ShareManagerInterface

func (c LonghornShareManagerClient) Create(shareManager *lhv1beta2.ShareManager) (*lhv1beta2.ShareManager, error) {
	return c(shareManager.Namespace).Create(context.TODO(), shareManager, metav1.CreateOptions{})
}

func (c LonghornShareManagerClient) Update(shareManager *lhv1beta2.ShareManager) (*lhv1beta2.ShareManager, error) {
	return c(shareManager.Namespace).Update(context.TODO(), shareManager, metav1.UpdateOptions{})
}

func (c LonghornShareManagerClient) UpdateStatus(_ *lhv1beta2.ShareManager) (*lhv1beta2.ShareManager, error) {
	panic("implement me")
}

func (c LonghornShareManagerClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c LonghornShareManagerClient) Get(namespace, name string, options metav1.GetOptions) (*lhv1beta2.ShareManager, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c LonghornShareManagerClient) List(namespace string, opts metav1.ListOptions) (*lhv1beta2.ShareManagerList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c LonghornShareManagerClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c LonghornShareManagerClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *lhv1beta2.ShareManager, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c LonghornShareManagerClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*lhv1beta2.ShareManager, *lhv1beta2.ShareManagerList], error) {
	panic("implement me")
}

type LonghornShareManagerCache func(string) longhornv1beta2.ShareManagerInterface

func (c LonghornShareManagerCache) Get(namespace, name string) (*lhv1beta2.ShareManager, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c LonghornShareManagerCache) List(namespace string, selector labels.Selector) ([]*lhv1beta2.ShareManager, error) {
	shareManagerList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*lhv1beta2.ShareManager, 0, len(shareManagerList.Items))
	for i := range shareManagerList.Items {
		result = append(result, &shareManagerList.Items[i])
	}

	return result, nil
}

func (c LonghornShareManagerCache) AddIndexer(_ string, _ generic.Indexer[*lhv1beta2.ShareManager]) {
	panic("implement me")
}

func (c LonghornShareManagerCache) GetByIndex(_, _ string) ([]*lhv1beta2.ShareManager, error) {
	panic("implement me")
}

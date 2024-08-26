package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	storagev1type "k8s.io/client-go/kubernetes/typed/storage/v1"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
)

type StorageClassClient func() storagev1type.StorageClassInterface

func (c StorageClassClient) Create(s *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c StorageClassClient) Update(s *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c StorageClassClient) UpdateStatus(_ *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	panic("implement me")
}

func (c StorageClassClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c StorageClassClient) Get(name string, options metav1.GetOptions) (*storagev1.StorageClass, error) {
	return c().Get(context.TODO(), name, options)
}

func (c StorageClassClient) List(opts metav1.ListOptions) (*storagev1.StorageClassList, error) {
	return c().List(context.TODO(), opts)
}

func (c StorageClassClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c StorageClassClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *storagev1.StorageClass, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type StorageClassCache func() storagev1type.StorageClassInterface

func (c StorageClassCache) Get(name string) (*storagev1.StorageClass, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c StorageClassCache) List(selector labels.Selector) ([]*storagev1.StorageClass, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*storagev1.StorageClass, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c StorageClassClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*storagev1.StorageClass, *storagev1.StorageClassList], error) {
	//TODO implement me
	panic("implement me")
}

func (c StorageClassCache) AddIndexer(_ string, _ generic.Indexer[*storagev1.StorageClass]) {
	panic("implement me")
}

func (c StorageClassCache) GetByIndex(indexName, key string) ([]*storagev1.StorageClass, error) {
	switch indexName {
	case indexeresutil.StorageClassBySecretIndex:
		secretNS, secretName := ref.Parse(key)
		scList, err := c().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var scs []*storagev1.StorageClass
		for _, sc := range scList.Items {
			sc := sc
			if secretName == sc.Parameters[util.CSINodePublishSecretNameKey] && secretNS == sc.Parameters[util.CSINodePublishSecretNamespaceKey] {
				scs = append(scs, &sc)
			}
		}
		return scs, nil
	default:
		return nil, nil
	}
}

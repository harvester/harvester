package fakeclients

import (
	"context"

	ctlv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/ref"
)

type PersistentVolumeClaimClient func(string) v1.PersistentVolumeClaimInterface

func (c PersistentVolumeClaimClient) Create(volume *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return c(volume.Namespace).Create(context.TODO(), volume, metav1.CreateOptions{})
}

func (c PersistentVolumeClaimClient) Update(volume *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	return c(volume.Namespace).Update(context.TODO(), volume, metav1.UpdateOptions{})
}

func (c PersistentVolumeClaimClient) UpdateStatus(volume *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	panic("implement me")
}

func (c PersistentVolumeClaimClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c PersistentVolumeClaimClient) Get(namespace, name string, options metav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c PersistentVolumeClaimClient) List(namespace string, opts metav1.ListOptions) (*corev1.PersistentVolumeClaimList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c PersistentVolumeClaimClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c PersistentVolumeClaimClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *corev1.PersistentVolumeClaim, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type PersistentVolumeClaimCache func(string) v1.PersistentVolumeClaimInterface

func (c PersistentVolumeClaimCache) Get(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c PersistentVolumeClaimCache) List(namespace string, selector labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	panic("implement me")
}

func (c PersistentVolumeClaimCache) AddIndexer(indexName string, indexer ctlv1.PersistentVolumeClaimIndexer) {
	panic("implement me")
}

func (c PersistentVolumeClaimCache) GetByIndex(indexName, key string) ([]*corev1.PersistentVolumeClaim, error) {
	switch indexName {
	case indexeres.PVCByVMIndex:
		vmNamespace, _ := ref.Parse(key)
		pvcList, err := c(vmNamespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var pvcs []*corev1.PersistentVolumeClaim
		for _, pvc := range pvcList.Items {
			pvcs = append(pvcs, &pvc)
		}
		return pvcs, nil
	default:
		return nil, nil
	}
}

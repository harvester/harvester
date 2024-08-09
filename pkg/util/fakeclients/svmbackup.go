package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvestertype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/ref"
)

type SVMBackupClient func(string) harvestertype.ScheduleVMBackupInterface

func (c SVMBackupClient) Create(svmBackup *harvesterv1beta1.ScheduleVMBackup) (*harvesterv1beta1.ScheduleVMBackup, error) {
	return c(svmBackup.Namespace).Create(context.TODO(), svmBackup, metav1.CreateOptions{})
}

func (c SVMBackupClient) Update(svmBackup *harvesterv1beta1.ScheduleVMBackup) (*harvesterv1beta1.ScheduleVMBackup, error) {
	return c(svmBackup.Namespace).Update(context.TODO(), svmBackup, metav1.UpdateOptions{})
}

func (c SVMBackupClient) UpdateStatus(_ *harvesterv1beta1.ScheduleVMBackup) (*harvesterv1beta1.ScheduleVMBackup, error) {
	panic("implement me")
}

func (c SVMBackupClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c SVMBackupClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1beta1.ScheduleVMBackup, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c SVMBackupClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1beta1.ScheduleVMBackupList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c SVMBackupClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c SVMBackupClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1beta1.ScheduleVMBackup, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c SVMBackupClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1beta1.ScheduleVMBackup, *harvesterv1beta1.ScheduleVMBackupList], error) {
	panic("implement me")
}

type SVMBackupCache func(string) harvestertype.ScheduleVMBackupInterface

func (c SVMBackupCache) Get(namespace, name string) (*harvesterv1beta1.ScheduleVMBackup, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c SVMBackupCache) List(namespace string, selector labels.Selector) ([]*harvesterv1beta1.ScheduleVMBackup, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1beta1.ScheduleVMBackup, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c SVMBackupCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1beta1.ScheduleVMBackup]) {
	panic("implement me")
}

func (c SVMBackupCache) GetByIndex(indexName, key string) ([]*harvesterv1beta1.ScheduleVMBackup, error) {
	switch indexName {
	case indexeres.VMBackupBySourceVMNameIndex:
		vmNamespace, _ := ref.Parse(key)
		backupList, err := c(vmNamespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var backups []*harvesterv1beta1.ScheduleVMBackup
		for i := range backupList.Items {
			b := backupList.Items[i]
			if b.Name == key {
				backups = append(backups, &b)
			}
		}
		return backups, nil
	default:
		return nil, nil
	}
}

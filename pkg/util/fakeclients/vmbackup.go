package fakeclients

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvestertype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	harvesterv1ctl "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/ref"
)

type VMBackupClient func(string) harvestertype.VirtualMachineBackupInterface

func (c VMBackupClient) Create(vmBackup *harvesterv1beta1.VirtualMachineBackup) (*harvesterv1beta1.VirtualMachineBackup, error) {
	return c(vmBackup.Namespace).Create(context.TODO(), vmBackup, metav1.CreateOptions{})
}

func (c VMBackupClient) Update(volume *harvesterv1beta1.VirtualMachineBackup) (*harvesterv1beta1.VirtualMachineBackup, error) {
	return c(volume.Namespace).Update(context.TODO(), volume, metav1.UpdateOptions{})
}

func (c VMBackupClient) UpdateStatus(volume *harvesterv1beta1.VirtualMachineBackup) (*harvesterv1beta1.VirtualMachineBackup, error) {
	panic("implement me")
}

func (c VMBackupClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c VMBackupClient) Get(namespace, name string, options metav1.GetOptions) (*harvesterv1beta1.VirtualMachineBackup, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VMBackupClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1beta1.VirtualMachineBackupList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c VMBackupClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c VMBackupClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1beta1.VirtualMachineBackup, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type VMBackupCache func(string) harvestertype.VirtualMachineBackupInterface

func (c VMBackupCache) Get(namespace, name string) (*harvesterv1beta1.VirtualMachineBackup, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VMBackupCache) List(namespace string, selector labels.Selector) ([]*harvesterv1beta1.VirtualMachineBackup, error) {
	panic("implement me")
}

func (c VMBackupCache) AddIndexer(indexName string, indexer harvesterv1ctl.VirtualMachineBackupIndexer) {
	panic("implement me")
}

func (c VMBackupCache) GetByIndex(indexName, key string) ([]*harvesterv1beta1.VirtualMachineBackup, error) {
	switch indexName {
	case indexeres.VMBackupBySourceVMNameIndex:
		vmNamespace, _ := ref.Parse(key)
		backupList, err := c(vmNamespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var backups []*harvesterv1beta1.VirtualMachineBackup
		for _, b := range backupList.Items {
			if b.Spec.Source.Name == key {
				backups = append(backups, &b)
			}
		}
		return backups, nil
	default:
		return nil, nil
	}
}

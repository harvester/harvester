package fakeclients

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	//batchv1type "k8s.io/client-go/kubernetes/typed/batch/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"

	batchv1type "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/batch/v1"
)

type CronJobClient func(string) batchv1type.CronJobInterface

func (c CronJobClient) Update(cronJob *batchv1.CronJob) (*batchv1.CronJob, error) {
	return c(cronJob.Namespace).Update(context.TODO(), cronJob, metav1.UpdateOptions{})
}
func (c CronJobClient) Get(namespace, name string, options metav1.GetOptions) (*batchv1.CronJob, error) {
	return c(namespace).Get(context.TODO(), name, options)
}
func (c CronJobClient) Create(cronJob *batchv1.CronJob) (*batchv1.CronJob, error) {
	return c(cronJob.Namespace).Create(context.TODO(), cronJob, metav1.CreateOptions{})
}
func (c CronJobClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c CronJobClient) List(_ string, _ metav1.ListOptions) (*batchv1.CronJobList, error) {
	panic("implement me")
}
func (c CronJobClient) UpdateStatus(*batchv1.CronJob) (*batchv1.CronJob, error) {
	panic("implement me")
}
func (c CronJobClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c CronJobClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *batchv1.CronJob, err error) {
	panic("implement me")
}

func (c CronJobClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*batchv1.CronJob, *batchv1.CronJobList], error) {
	panic("implement me")
}

type CronJobCache func(string) batchv1type.CronJobInterface

func (c CronJobCache) Get(namespace, name string) (*batchv1.CronJob, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c CronJobCache) List(_ string, _ labels.Selector) ([]*batchv1.CronJob, error) {
	panic("implement me")
}

func (c CronJobCache) AddIndexer(_ string, _ generic.Indexer[*batchv1.CronJob]) {
	panic("implement me")
}

func (c CronJobCache) GetByIndex(_, _ string) ([]*batchv1.CronJob, error) {
	panic("implement me")
}

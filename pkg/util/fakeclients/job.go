package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	batchv1type "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
)

type JobCache func(string) batchv1type.JobInterface

func (c JobCache) Get(namespace, name string) (*batchv1.Job, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c JobCache) List(namespace string, selector labels.Selector) ([]*batchv1.Job, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*batchv1.Job, 0, len(list.Items))
	for _, job := range list.Items {
		obj := job
		result = append(result, &obj)
	}
	return result, err
}

func (c JobCache) AddIndexer(_ string, _ generic.Indexer[*batchv1.Job]) {
	panic("implement me")
}

func (c JobCache) GetByIndex(_, _ string) ([]*batchv1.Job, error) {
	panic("implement me")
}

type JobClient func(string) batchv1type.JobInterface

func (c JobClient) Update(job *batchv1.Job) (*batchv1.Job, error) {
	return c(job.Namespace).Update(context.TODO(), job, metav1.UpdateOptions{})
}
func (c JobClient) Get(_, _ string, _ metav1.GetOptions) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) Create(job *batchv1.Job) (*batchv1.Job, error) {
	return c(job.Namespace).Create(context.TODO(), job, metav1.CreateOptions{})
}
func (c JobClient) UpdateStatus(*batchv1.Job) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c JobClient) List(_ string, _ metav1.ListOptions) (*batchv1.JobList, error) {
	panic("implement me")
}
func (c JobClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c JobClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *batchv1.Job, err error) {
	panic("implement me")
}
func (c JobClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*batchv1.Job, *batchv1.JobList], error) {
	panic("implement me")
}

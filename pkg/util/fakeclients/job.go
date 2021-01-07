package fakeclients

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	batchv1type "k8s.io/client-go/kubernetes/typed/batch/v1"
)

type JobClient func(string) batchv1type.JobInterface

func (c JobClient) Update(job *batchv1.Job) (*batchv1.Job, error) {
	return c(job.Namespace).Update(context.TODO(), job, metav1.UpdateOptions{})
}
func (c JobClient) Get(namespace, name string, options metav1.GetOptions) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) Create(*batchv1.Job) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) UpdateStatus(*batchv1.Job) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c JobClient) List(namespace string, opts metav1.ListOptions) (*batchv1.Job, error) {
	panic("implement me")
}
func (c JobClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c JobClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *batchv1.Job, err error) {
	panic("implement me")
}

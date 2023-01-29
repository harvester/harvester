package metrics

import (
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type ResourceClientWithMetrics struct {
	dynamic.ResourceInterface
}

func Wrap(resourceInterface dynamic.ResourceInterface, err error) (ResourceClientWithMetrics, error) {
	client := ResourceClientWithMetrics{}
	if err != nil {
		return client, err
	}
	client.ResourceInterface = resourceInterface
	return client, err
}

func (r ResourceClientWithMetrics) Create(apiOp *types.APIRequest, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	obj, err := r.ResourceInterface.Create(apiOp.Context(), obj, options, subresources...)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return obj, err
}

func (r ResourceClientWithMetrics) Update(apiOp *types.APIRequest, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	obj, err := r.ResourceInterface.Update(apiOp.Context(), obj, options, subresources...)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return obj, err
}

func (r ResourceClientWithMetrics) Delete(apiOp *types.APIRequest, name string, options metav1.DeleteOptions, subresources ...string) error {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	err := r.ResourceInterface.Delete(apiOp.Context(), name, options, subresources...)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return err
}

func (r ResourceClientWithMetrics) Get(apiOp *types.APIRequest, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	obj, err := r.ResourceInterface.Get(apiOp.Context(), name, options, subresources...)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return obj, err
}

func (r ResourceClientWithMetrics) List(apiOp *types.APIRequest, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	obj, err := r.ResourceInterface.List(apiOp.Context(), opts)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return obj, err
}

func (r ResourceClientWithMetrics) Watch(apiOp *types.APIRequest, opts metav1.ListOptions) (watch.Interface, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	watchInterface, err := r.ResourceInterface.Watch(apiOp.Context(), opts)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return watchInterface, err
}

func (r ResourceClientWithMetrics) Patch(apiOp *types.APIRequest, name string, pt k8stypes.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	m := metrics.MetricLogger{Resource: apiOp.Schema.ID, Method: apiOp.Method}
	start := time.Now()
	obj, err := r.ResourceInterface.Patch(apiOp.Context(), name, pt, data, options, subresources...)
	m.RecordK8sClientResponseTime(err, float64(time.Since(start).Milliseconds()))
	return obj, err
}

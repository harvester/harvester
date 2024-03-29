/*
Copyright 2024 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package fake

import (
	"context"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeMonitorMetrics implements MonitorMetricInterface
type FakeMonitorMetrics struct {
	Fake *FakeManagementV3
	ns   string
}

var monitormetricsResource = schema.GroupVersionResource{Group: "management.cattle.io", Version: "v3", Resource: "monitormetrics"}

var monitormetricsKind = schema.GroupVersionKind{Group: "management.cattle.io", Version: "v3", Kind: "MonitorMetric"}

// Get takes name of the monitorMetric, and returns the corresponding monitorMetric object, and an error if there is any.
func (c *FakeMonitorMetrics) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.MonitorMetric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(monitormetricsResource, c.ns, name), &v3.MonitorMetric{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.MonitorMetric), err
}

// List takes label and field selectors, and returns the list of MonitorMetrics that match those selectors.
func (c *FakeMonitorMetrics) List(ctx context.Context, opts v1.ListOptions) (result *v3.MonitorMetricList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(monitormetricsResource, monitormetricsKind, c.ns, opts), &v3.MonitorMetricList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.MonitorMetricList{ListMeta: obj.(*v3.MonitorMetricList).ListMeta}
	for _, item := range obj.(*v3.MonitorMetricList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested monitorMetrics.
func (c *FakeMonitorMetrics) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(monitormetricsResource, c.ns, opts))

}

// Create takes the representation of a monitorMetric and creates it.  Returns the server's representation of the monitorMetric, and an error, if there is any.
func (c *FakeMonitorMetrics) Create(ctx context.Context, monitorMetric *v3.MonitorMetric, opts v1.CreateOptions) (result *v3.MonitorMetric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(monitormetricsResource, c.ns, monitorMetric), &v3.MonitorMetric{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.MonitorMetric), err
}

// Update takes the representation of a monitorMetric and updates it. Returns the server's representation of the monitorMetric, and an error, if there is any.
func (c *FakeMonitorMetrics) Update(ctx context.Context, monitorMetric *v3.MonitorMetric, opts v1.UpdateOptions) (result *v3.MonitorMetric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(monitormetricsResource, c.ns, monitorMetric), &v3.MonitorMetric{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.MonitorMetric), err
}

// Delete takes name of the monitorMetric and deletes it. Returns an error if one occurs.
func (c *FakeMonitorMetrics) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(monitormetricsResource, c.ns, name, opts), &v3.MonitorMetric{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeMonitorMetrics) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(monitormetricsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v3.MonitorMetricList{})
	return err
}

// Patch applies the patch and returns the patched monitorMetric.
func (c *FakeMonitorMetrics) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.MonitorMetric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(monitormetricsResource, c.ns, name, pt, data, subresources...), &v3.MonitorMetric{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.MonitorMetric), err
}

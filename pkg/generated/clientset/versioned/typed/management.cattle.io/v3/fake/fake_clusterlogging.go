/*
Copyright 2025 Rancher Labs, Inc.

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
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeClusterLoggings implements ClusterLoggingInterface
type FakeClusterLoggings struct {
	Fake *FakeManagementV3
	ns   string
}

var clusterloggingsResource = v3.SchemeGroupVersion.WithResource("clusterloggings")

var clusterloggingsKind = v3.SchemeGroupVersion.WithKind("ClusterLogging")

// Get takes name of the clusterLogging, and returns the corresponding clusterLogging object, and an error if there is any.
func (c *FakeClusterLoggings) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.ClusterLogging, err error) {
	emptyResult := &v3.ClusterLogging{}
	obj, err := c.Fake.
		Invokes(testing.NewGetActionWithOptions(clusterloggingsResource, c.ns, name, options), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.ClusterLogging), err
}

// List takes label and field selectors, and returns the list of ClusterLoggings that match those selectors.
func (c *FakeClusterLoggings) List(ctx context.Context, opts v1.ListOptions) (result *v3.ClusterLoggingList, err error) {
	emptyResult := &v3.ClusterLoggingList{}
	obj, err := c.Fake.
		Invokes(testing.NewListActionWithOptions(clusterloggingsResource, clusterloggingsKind, c.ns, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.ClusterLoggingList{ListMeta: obj.(*v3.ClusterLoggingList).ListMeta}
	for _, item := range obj.(*v3.ClusterLoggingList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusterLoggings.
func (c *FakeClusterLoggings) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchActionWithOptions(clusterloggingsResource, c.ns, opts))

}

// Create takes the representation of a clusterLogging and creates it.  Returns the server's representation of the clusterLogging, and an error, if there is any.
func (c *FakeClusterLoggings) Create(ctx context.Context, clusterLogging *v3.ClusterLogging, opts v1.CreateOptions) (result *v3.ClusterLogging, err error) {
	emptyResult := &v3.ClusterLogging{}
	obj, err := c.Fake.
		Invokes(testing.NewCreateActionWithOptions(clusterloggingsResource, c.ns, clusterLogging, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.ClusterLogging), err
}

// Update takes the representation of a clusterLogging and updates it. Returns the server's representation of the clusterLogging, and an error, if there is any.
func (c *FakeClusterLoggings) Update(ctx context.Context, clusterLogging *v3.ClusterLogging, opts v1.UpdateOptions) (result *v3.ClusterLogging, err error) {
	emptyResult := &v3.ClusterLogging{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateActionWithOptions(clusterloggingsResource, c.ns, clusterLogging, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.ClusterLogging), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeClusterLoggings) UpdateStatus(ctx context.Context, clusterLogging *v3.ClusterLogging, opts v1.UpdateOptions) (result *v3.ClusterLogging, err error) {
	emptyResult := &v3.ClusterLogging{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceActionWithOptions(clusterloggingsResource, "status", c.ns, clusterLogging, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.ClusterLogging), err
}

// Delete takes name of the clusterLogging and deletes it. Returns an error if one occurs.
func (c *FakeClusterLoggings) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(clusterloggingsResource, c.ns, name, opts), &v3.ClusterLogging{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusterLoggings) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionActionWithOptions(clusterloggingsResource, c.ns, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v3.ClusterLoggingList{})
	return err
}

// Patch applies the patch and returns the patched clusterLogging.
func (c *FakeClusterLoggings) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.ClusterLogging, err error) {
	emptyResult := &v3.ClusterLogging{}
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceActionWithOptions(clusterloggingsResource, c.ns, name, pt, data, opts, subresources...), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.ClusterLogging), err
}

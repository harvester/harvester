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

// FakeProjectAlerts implements ProjectAlertInterface
type FakeProjectAlerts struct {
	Fake *FakeManagementV3
	ns   string
}

var projectalertsResource = schema.GroupVersionResource{Group: "management.cattle.io", Version: "v3", Resource: "projectalerts"}

var projectalertsKind = schema.GroupVersionKind{Group: "management.cattle.io", Version: "v3", Kind: "ProjectAlert"}

// Get takes name of the projectAlert, and returns the corresponding projectAlert object, and an error if there is any.
func (c *FakeProjectAlerts) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.ProjectAlert, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(projectalertsResource, c.ns, name), &v3.ProjectAlert{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlert), err
}

// List takes label and field selectors, and returns the list of ProjectAlerts that match those selectors.
func (c *FakeProjectAlerts) List(ctx context.Context, opts v1.ListOptions) (result *v3.ProjectAlertList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(projectalertsResource, projectalertsKind, c.ns, opts), &v3.ProjectAlertList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.ProjectAlertList{ListMeta: obj.(*v3.ProjectAlertList).ListMeta}
	for _, item := range obj.(*v3.ProjectAlertList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested projectAlerts.
func (c *FakeProjectAlerts) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(projectalertsResource, c.ns, opts))

}

// Create takes the representation of a projectAlert and creates it.  Returns the server's representation of the projectAlert, and an error, if there is any.
func (c *FakeProjectAlerts) Create(ctx context.Context, projectAlert *v3.ProjectAlert, opts v1.CreateOptions) (result *v3.ProjectAlert, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(projectalertsResource, c.ns, projectAlert), &v3.ProjectAlert{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlert), err
}

// Update takes the representation of a projectAlert and updates it. Returns the server's representation of the projectAlert, and an error, if there is any.
func (c *FakeProjectAlerts) Update(ctx context.Context, projectAlert *v3.ProjectAlert, opts v1.UpdateOptions) (result *v3.ProjectAlert, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(projectalertsResource, c.ns, projectAlert), &v3.ProjectAlert{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlert), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeProjectAlerts) UpdateStatus(ctx context.Context, projectAlert *v3.ProjectAlert, opts v1.UpdateOptions) (*v3.ProjectAlert, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(projectalertsResource, "status", c.ns, projectAlert), &v3.ProjectAlert{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlert), err
}

// Delete takes name of the projectAlert and deletes it. Returns an error if one occurs.
func (c *FakeProjectAlerts) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(projectalertsResource, c.ns, name, opts), &v3.ProjectAlert{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeProjectAlerts) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(projectalertsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v3.ProjectAlertList{})
	return err
}

// Patch applies the patch and returns the patched projectAlert.
func (c *FakeProjectAlerts) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.ProjectAlert, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(projectalertsResource, c.ns, name, pt, data, subresources...), &v3.ProjectAlert{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlert), err
}
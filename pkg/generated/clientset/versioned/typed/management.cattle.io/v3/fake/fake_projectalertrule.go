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

// FakeProjectAlertRules implements ProjectAlertRuleInterface
type FakeProjectAlertRules struct {
	Fake *FakeManagementV3
	ns   string
}

var projectalertrulesResource = schema.GroupVersionResource{Group: "management.cattle.io", Version: "v3", Resource: "projectalertrules"}

var projectalertrulesKind = schema.GroupVersionKind{Group: "management.cattle.io", Version: "v3", Kind: "ProjectAlertRule"}

// Get takes name of the projectAlertRule, and returns the corresponding projectAlertRule object, and an error if there is any.
func (c *FakeProjectAlertRules) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.ProjectAlertRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(projectalertrulesResource, c.ns, name), &v3.ProjectAlertRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlertRule), err
}

// List takes label and field selectors, and returns the list of ProjectAlertRules that match those selectors.
func (c *FakeProjectAlertRules) List(ctx context.Context, opts v1.ListOptions) (result *v3.ProjectAlertRuleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(projectalertrulesResource, projectalertrulesKind, c.ns, opts), &v3.ProjectAlertRuleList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.ProjectAlertRuleList{ListMeta: obj.(*v3.ProjectAlertRuleList).ListMeta}
	for _, item := range obj.(*v3.ProjectAlertRuleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested projectAlertRules.
func (c *FakeProjectAlertRules) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(projectalertrulesResource, c.ns, opts))

}

// Create takes the representation of a projectAlertRule and creates it.  Returns the server's representation of the projectAlertRule, and an error, if there is any.
func (c *FakeProjectAlertRules) Create(ctx context.Context, projectAlertRule *v3.ProjectAlertRule, opts v1.CreateOptions) (result *v3.ProjectAlertRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(projectalertrulesResource, c.ns, projectAlertRule), &v3.ProjectAlertRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlertRule), err
}

// Update takes the representation of a projectAlertRule and updates it. Returns the server's representation of the projectAlertRule, and an error, if there is any.
func (c *FakeProjectAlertRules) Update(ctx context.Context, projectAlertRule *v3.ProjectAlertRule, opts v1.UpdateOptions) (result *v3.ProjectAlertRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(projectalertrulesResource, c.ns, projectAlertRule), &v3.ProjectAlertRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlertRule), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeProjectAlertRules) UpdateStatus(ctx context.Context, projectAlertRule *v3.ProjectAlertRule, opts v1.UpdateOptions) (*v3.ProjectAlertRule, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(projectalertrulesResource, "status", c.ns, projectAlertRule), &v3.ProjectAlertRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlertRule), err
}

// Delete takes name of the projectAlertRule and deletes it. Returns an error if one occurs.
func (c *FakeProjectAlertRules) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(projectalertrulesResource, c.ns, name, opts), &v3.ProjectAlertRule{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeProjectAlertRules) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(projectalertrulesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v3.ProjectAlertRuleList{})
	return err
}

// Patch applies the patch and returns the patched projectAlertRule.
func (c *FakeProjectAlertRules) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.ProjectAlertRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(projectalertrulesResource, c.ns, name, pt, data, subresources...), &v3.ProjectAlertRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v3.ProjectAlertRule), err
}

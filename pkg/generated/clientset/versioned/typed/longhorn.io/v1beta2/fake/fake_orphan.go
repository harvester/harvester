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

	v1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeOrphans implements OrphanInterface
type FakeOrphans struct {
	Fake *FakeLonghornV1beta2
	ns   string
}

var orphansResource = v1beta2.SchemeGroupVersion.WithResource("orphans")

var orphansKind = v1beta2.SchemeGroupVersion.WithKind("Orphan")

// Get takes name of the orphan, and returns the corresponding orphan object, and an error if there is any.
func (c *FakeOrphans) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta2.Orphan, err error) {
	emptyResult := &v1beta2.Orphan{}
	obj, err := c.Fake.
		Invokes(testing.NewGetActionWithOptions(orphansResource, c.ns, name, options), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.Orphan), err
}

// List takes label and field selectors, and returns the list of Orphans that match those selectors.
func (c *FakeOrphans) List(ctx context.Context, opts v1.ListOptions) (result *v1beta2.OrphanList, err error) {
	emptyResult := &v1beta2.OrphanList{}
	obj, err := c.Fake.
		Invokes(testing.NewListActionWithOptions(orphansResource, orphansKind, c.ns, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta2.OrphanList{ListMeta: obj.(*v1beta2.OrphanList).ListMeta}
	for _, item := range obj.(*v1beta2.OrphanList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested orphans.
func (c *FakeOrphans) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchActionWithOptions(orphansResource, c.ns, opts))

}

// Create takes the representation of a orphan and creates it.  Returns the server's representation of the orphan, and an error, if there is any.
func (c *FakeOrphans) Create(ctx context.Context, orphan *v1beta2.Orphan, opts v1.CreateOptions) (result *v1beta2.Orphan, err error) {
	emptyResult := &v1beta2.Orphan{}
	obj, err := c.Fake.
		Invokes(testing.NewCreateActionWithOptions(orphansResource, c.ns, orphan, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.Orphan), err
}

// Update takes the representation of a orphan and updates it. Returns the server's representation of the orphan, and an error, if there is any.
func (c *FakeOrphans) Update(ctx context.Context, orphan *v1beta2.Orphan, opts v1.UpdateOptions) (result *v1beta2.Orphan, err error) {
	emptyResult := &v1beta2.Orphan{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateActionWithOptions(orphansResource, c.ns, orphan, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.Orphan), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeOrphans) UpdateStatus(ctx context.Context, orphan *v1beta2.Orphan, opts v1.UpdateOptions) (result *v1beta2.Orphan, err error) {
	emptyResult := &v1beta2.Orphan{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceActionWithOptions(orphansResource, "status", c.ns, orphan, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.Orphan), err
}

// Delete takes name of the orphan and deletes it. Returns an error if one occurs.
func (c *FakeOrphans) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(orphansResource, c.ns, name, opts), &v1beta2.Orphan{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeOrphans) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionActionWithOptions(orphansResource, c.ns, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta2.OrphanList{})
	return err
}

// Patch applies the patch and returns the patched orphan.
func (c *FakeOrphans) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta2.Orphan, err error) {
	emptyResult := &v1beta2.Orphan{}
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceActionWithOptions(orphansResource, c.ns, name, pt, data, opts, subresources...), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.Orphan), err
}

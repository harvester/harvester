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

	v1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeBackupBackingImages implements BackupBackingImageInterface
type FakeBackupBackingImages struct {
	Fake *FakeLonghornV1beta2
	ns   string
}

var backupbackingimagesResource = schema.GroupVersionResource{Group: "longhorn.io", Version: "v1beta2", Resource: "backupbackingimages"}

var backupbackingimagesKind = schema.GroupVersionKind{Group: "longhorn.io", Version: "v1beta2", Kind: "BackupBackingImage"}

// Get takes name of the backupBackingImage, and returns the corresponding backupBackingImage object, and an error if there is any.
func (c *FakeBackupBackingImages) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta2.BackupBackingImage, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(backupbackingimagesResource, c.ns, name), &v1beta2.BackupBackingImage{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.BackupBackingImage), err
}

// List takes label and field selectors, and returns the list of BackupBackingImages that match those selectors.
func (c *FakeBackupBackingImages) List(ctx context.Context, opts v1.ListOptions) (result *v1beta2.BackupBackingImageList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(backupbackingimagesResource, backupbackingimagesKind, c.ns, opts), &v1beta2.BackupBackingImageList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta2.BackupBackingImageList{ListMeta: obj.(*v1beta2.BackupBackingImageList).ListMeta}
	for _, item := range obj.(*v1beta2.BackupBackingImageList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested backupBackingImages.
func (c *FakeBackupBackingImages) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(backupbackingimagesResource, c.ns, opts))

}

// Create takes the representation of a backupBackingImage and creates it.  Returns the server's representation of the backupBackingImage, and an error, if there is any.
func (c *FakeBackupBackingImages) Create(ctx context.Context, backupBackingImage *v1beta2.BackupBackingImage, opts v1.CreateOptions) (result *v1beta2.BackupBackingImage, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(backupbackingimagesResource, c.ns, backupBackingImage), &v1beta2.BackupBackingImage{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.BackupBackingImage), err
}

// Update takes the representation of a backupBackingImage and updates it. Returns the server's representation of the backupBackingImage, and an error, if there is any.
func (c *FakeBackupBackingImages) Update(ctx context.Context, backupBackingImage *v1beta2.BackupBackingImage, opts v1.UpdateOptions) (result *v1beta2.BackupBackingImage, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(backupbackingimagesResource, c.ns, backupBackingImage), &v1beta2.BackupBackingImage{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.BackupBackingImage), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeBackupBackingImages) UpdateStatus(ctx context.Context, backupBackingImage *v1beta2.BackupBackingImage, opts v1.UpdateOptions) (*v1beta2.BackupBackingImage, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(backupbackingimagesResource, "status", c.ns, backupBackingImage), &v1beta2.BackupBackingImage{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.BackupBackingImage), err
}

// Delete takes name of the backupBackingImage and deletes it. Returns an error if one occurs.
func (c *FakeBackupBackingImages) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(backupbackingimagesResource, c.ns, name, opts), &v1beta2.BackupBackingImage{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeBackupBackingImages) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(backupbackingimagesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta2.BackupBackingImageList{})
	return err
}

// Patch applies the patch and returns the patched backupBackingImage.
func (c *FakeBackupBackingImages) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta2.BackupBackingImage, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(backupbackingimagesResource, c.ns, name, pt, data, subresources...), &v1beta2.BackupBackingImage{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.BackupBackingImage), err
}
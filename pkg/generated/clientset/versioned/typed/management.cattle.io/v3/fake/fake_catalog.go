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

// FakeCatalogs implements CatalogInterface
type FakeCatalogs struct {
	Fake *FakeManagementV3
}

var catalogsResource = v3.SchemeGroupVersion.WithResource("catalogs")

var catalogsKind = v3.SchemeGroupVersion.WithKind("Catalog")

// Get takes name of the catalog, and returns the corresponding catalog object, and an error if there is any.
func (c *FakeCatalogs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.Catalog, err error) {
	emptyResult := &v3.Catalog{}
	obj, err := c.Fake.
		Invokes(testing.NewRootGetActionWithOptions(catalogsResource, name, options), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.Catalog), err
}

// List takes label and field selectors, and returns the list of Catalogs that match those selectors.
func (c *FakeCatalogs) List(ctx context.Context, opts v1.ListOptions) (result *v3.CatalogList, err error) {
	emptyResult := &v3.CatalogList{}
	obj, err := c.Fake.
		Invokes(testing.NewRootListActionWithOptions(catalogsResource, catalogsKind, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.CatalogList{ListMeta: obj.(*v3.CatalogList).ListMeta}
	for _, item := range obj.(*v3.CatalogList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested catalogs.
func (c *FakeCatalogs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchActionWithOptions(catalogsResource, opts))
}

// Create takes the representation of a catalog and creates it.  Returns the server's representation of the catalog, and an error, if there is any.
func (c *FakeCatalogs) Create(ctx context.Context, catalog *v3.Catalog, opts v1.CreateOptions) (result *v3.Catalog, err error) {
	emptyResult := &v3.Catalog{}
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateActionWithOptions(catalogsResource, catalog, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.Catalog), err
}

// Update takes the representation of a catalog and updates it. Returns the server's representation of the catalog, and an error, if there is any.
func (c *FakeCatalogs) Update(ctx context.Context, catalog *v3.Catalog, opts v1.UpdateOptions) (result *v3.Catalog, err error) {
	emptyResult := &v3.Catalog{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateActionWithOptions(catalogsResource, catalog, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.Catalog), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeCatalogs) UpdateStatus(ctx context.Context, catalog *v3.Catalog, opts v1.UpdateOptions) (result *v3.Catalog, err error) {
	emptyResult := &v3.Catalog{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceActionWithOptions(catalogsResource, "status", catalog, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.Catalog), err
}

// Delete takes name of the catalog and deletes it. Returns an error if one occurs.
func (c *FakeCatalogs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(catalogsResource, name, opts), &v3.Catalog{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeCatalogs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionActionWithOptions(catalogsResource, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v3.CatalogList{})
	return err
}

// Patch applies the patch and returns the patched catalog.
func (c *FakeCatalogs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.Catalog, err error) {
	emptyResult := &v3.Catalog{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(catalogsResource, name, pt, data, opts, subresources...), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.Catalog), err
}

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

package v3

import (
	"context"

	scheme "github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// UsersGetter has a method to return a UserInterface.
// A group's client should implement this interface.
type UsersGetter interface {
	Users() UserInterface
}

// UserInterface has methods to work with User resources.
type UserInterface interface {
	Create(ctx context.Context, user *v3.User, opts v1.CreateOptions) (*v3.User, error)
	Update(ctx context.Context, user *v3.User, opts v1.UpdateOptions) (*v3.User, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, user *v3.User, opts v1.UpdateOptions) (*v3.User, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v3.User, error)
	List(ctx context.Context, opts v1.ListOptions) (*v3.UserList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.User, err error)
	UserExpansion
}

// users implements UserInterface
type users struct {
	*gentype.ClientWithList[*v3.User, *v3.UserList]
}

// newUsers returns a Users
func newUsers(c *ManagementV3Client) *users {
	return &users{
		gentype.NewClientWithList[*v3.User, *v3.UserList](
			"users",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *v3.User { return &v3.User{} },
			func() *v3.UserList { return &v3.UserList{} }),
	}
}

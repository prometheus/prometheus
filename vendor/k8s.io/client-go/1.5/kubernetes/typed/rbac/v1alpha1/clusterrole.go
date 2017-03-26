/*
Copyright 2016 The Kubernetes Authors.

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

package v1alpha1

import (
	api "k8s.io/client-go/1.5/pkg/api"
	v1alpha1 "k8s.io/client-go/1.5/pkg/apis/rbac/v1alpha1"
	watch "k8s.io/client-go/1.5/pkg/watch"
)

// ClusterRolesGetter has a method to return a ClusterRoleInterface.
// A group's client should implement this interface.
type ClusterRolesGetter interface {
	ClusterRoles() ClusterRoleInterface
}

// ClusterRoleInterface has methods to work with ClusterRole resources.
type ClusterRoleInterface interface {
	Create(*v1alpha1.ClusterRole) (*v1alpha1.ClusterRole, error)
	Update(*v1alpha1.ClusterRole) (*v1alpha1.ClusterRole, error)
	Delete(name string, options *api.DeleteOptions) error
	DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error
	Get(name string) (*v1alpha1.ClusterRole, error)
	List(opts api.ListOptions) (*v1alpha1.ClusterRoleList, error)
	Watch(opts api.ListOptions) (watch.Interface, error)
	Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterRole, err error)
	ClusterRoleExpansion
}

// clusterRoles implements ClusterRoleInterface
type clusterRoles struct {
	client *RbacClient
}

// newClusterRoles returns a ClusterRoles
func newClusterRoles(c *RbacClient) *clusterRoles {
	return &clusterRoles{
		client: c,
	}
}

// Create takes the representation of a clusterRole and creates it.  Returns the server's representation of the clusterRole, and an error, if there is any.
func (c *clusterRoles) Create(clusterRole *v1alpha1.ClusterRole) (result *v1alpha1.ClusterRole, err error) {
	result = &v1alpha1.ClusterRole{}
	err = c.client.Post().
		Resource("clusterroles").
		Body(clusterRole).
		Do().
		Into(result)
	return
}

// Update takes the representation of a clusterRole and updates it. Returns the server's representation of the clusterRole, and an error, if there is any.
func (c *clusterRoles) Update(clusterRole *v1alpha1.ClusterRole) (result *v1alpha1.ClusterRole, err error) {
	result = &v1alpha1.ClusterRole{}
	err = c.client.Put().
		Resource("clusterroles").
		Name(clusterRole.Name).
		Body(clusterRole).
		Do().
		Into(result)
	return
}

// Delete takes name of the clusterRole and deletes it. Returns an error if one occurs.
func (c *clusterRoles) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusterroles").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterRoles) DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error {
	return c.client.Delete().
		Resource("clusterroles").
		VersionedParams(&listOptions, api.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the clusterRole, and returns the corresponding clusterRole object, and an error if there is any.
func (c *clusterRoles) Get(name string) (result *v1alpha1.ClusterRole, err error) {
	result = &v1alpha1.ClusterRole{}
	err = c.client.Get().
		Resource("clusterroles").
		Name(name).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterRoles that match those selectors.
func (c *clusterRoles) List(opts api.ListOptions) (result *v1alpha1.ClusterRoleList, err error) {
	result = &v1alpha1.ClusterRoleList{}
	err = c.client.Get().
		Resource("clusterroles").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterRoles.
func (c *clusterRoles) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Prefix("watch").
		Resource("clusterroles").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched clusterRole.
func (c *clusterRoles) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterRole, err error) {
	result = &v1alpha1.ClusterRole{}
	err = c.client.Patch(pt).
		Resource("clusterroles").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

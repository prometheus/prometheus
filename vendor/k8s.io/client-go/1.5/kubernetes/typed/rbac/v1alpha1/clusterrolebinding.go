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

// ClusterRoleBindingsGetter has a method to return a ClusterRoleBindingInterface.
// A group's client should implement this interface.
type ClusterRoleBindingsGetter interface {
	ClusterRoleBindings() ClusterRoleBindingInterface
}

// ClusterRoleBindingInterface has methods to work with ClusterRoleBinding resources.
type ClusterRoleBindingInterface interface {
	Create(*v1alpha1.ClusterRoleBinding) (*v1alpha1.ClusterRoleBinding, error)
	Update(*v1alpha1.ClusterRoleBinding) (*v1alpha1.ClusterRoleBinding, error)
	Delete(name string, options *api.DeleteOptions) error
	DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error
	Get(name string) (*v1alpha1.ClusterRoleBinding, error)
	List(opts api.ListOptions) (*v1alpha1.ClusterRoleBindingList, error)
	Watch(opts api.ListOptions) (watch.Interface, error)
	Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterRoleBinding, err error)
	ClusterRoleBindingExpansion
}

// clusterRoleBindings implements ClusterRoleBindingInterface
type clusterRoleBindings struct {
	client *RbacClient
}

// newClusterRoleBindings returns a ClusterRoleBindings
func newClusterRoleBindings(c *RbacClient) *clusterRoleBindings {
	return &clusterRoleBindings{
		client: c,
	}
}

// Create takes the representation of a clusterRoleBinding and creates it.  Returns the server's representation of the clusterRoleBinding, and an error, if there is any.
func (c *clusterRoleBindings) Create(clusterRoleBinding *v1alpha1.ClusterRoleBinding) (result *v1alpha1.ClusterRoleBinding, err error) {
	result = &v1alpha1.ClusterRoleBinding{}
	err = c.client.Post().
		Resource("clusterrolebindings").
		Body(clusterRoleBinding).
		Do().
		Into(result)
	return
}

// Update takes the representation of a clusterRoleBinding and updates it. Returns the server's representation of the clusterRoleBinding, and an error, if there is any.
func (c *clusterRoleBindings) Update(clusterRoleBinding *v1alpha1.ClusterRoleBinding) (result *v1alpha1.ClusterRoleBinding, err error) {
	result = &v1alpha1.ClusterRoleBinding{}
	err = c.client.Put().
		Resource("clusterrolebindings").
		Name(clusterRoleBinding.Name).
		Body(clusterRoleBinding).
		Do().
		Into(result)
	return
}

// Delete takes name of the clusterRoleBinding and deletes it. Returns an error if one occurs.
func (c *clusterRoleBindings) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusterrolebindings").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterRoleBindings) DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error {
	return c.client.Delete().
		Resource("clusterrolebindings").
		VersionedParams(&listOptions, api.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the clusterRoleBinding, and returns the corresponding clusterRoleBinding object, and an error if there is any.
func (c *clusterRoleBindings) Get(name string) (result *v1alpha1.ClusterRoleBinding, err error) {
	result = &v1alpha1.ClusterRoleBinding{}
	err = c.client.Get().
		Resource("clusterrolebindings").
		Name(name).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterRoleBindings that match those selectors.
func (c *clusterRoleBindings) List(opts api.ListOptions) (result *v1alpha1.ClusterRoleBindingList, err error) {
	result = &v1alpha1.ClusterRoleBindingList{}
	err = c.client.Get().
		Resource("clusterrolebindings").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterRoleBindings.
func (c *clusterRoleBindings) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Prefix("watch").
		Resource("clusterrolebindings").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched clusterRoleBinding.
func (c *clusterRoleBindings) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterRoleBinding, err error) {
	result = &v1alpha1.ClusterRoleBinding{}
	err = c.client.Patch(pt).
		Resource("clusterrolebindings").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

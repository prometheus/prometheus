/*
Copyright 2018 The Kubernetes Authors.

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

package v1

import (
	v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
)

// ClusterRoleBindingsGetter has a method to return a ClusterRoleBindingInterface.
// A group's client should implement this interface.
type ClusterRoleBindingsGetter interface {
	ClusterRoleBindings() ClusterRoleBindingInterface
}

// ClusterRoleBindingInterface has methods to work with ClusterRoleBinding resources.
type ClusterRoleBindingInterface interface {
	Create(*v1.ClusterRoleBinding) (*v1.ClusterRoleBinding, error)
	Update(*v1.ClusterRoleBinding) (*v1.ClusterRoleBinding, error)
	Delete(name string, options *meta_v1.DeleteOptions) error
	DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error
	Get(name string, options meta_v1.GetOptions) (*v1.ClusterRoleBinding, error)
	List(opts meta_v1.ListOptions) (*v1.ClusterRoleBindingList, error)
	Watch(opts meta_v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ClusterRoleBinding, err error)
	ClusterRoleBindingExpansion
}

// clusterRoleBindings implements ClusterRoleBindingInterface
type clusterRoleBindings struct {
	client rest.Interface
}

// newClusterRoleBindings returns a ClusterRoleBindings
func newClusterRoleBindings(c *RbacV1Client) *clusterRoleBindings {
	return &clusterRoleBindings{
		client: c.RESTClient(),
	}
}

// Get takes name of the clusterRoleBinding, and returns the corresponding clusterRoleBinding object, and an error if there is any.
func (c *clusterRoleBindings) Get(name string, options meta_v1.GetOptions) (result *v1.ClusterRoleBinding, err error) {
	result = &v1.ClusterRoleBinding{}
	err = c.client.Get().
		Resource("clusterrolebindings").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterRoleBindings that match those selectors.
func (c *clusterRoleBindings) List(opts meta_v1.ListOptions) (result *v1.ClusterRoleBindingList, err error) {
	result = &v1.ClusterRoleBindingList{}
	err = c.client.Get().
		Resource("clusterrolebindings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterRoleBindings.
func (c *clusterRoleBindings) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Resource("clusterrolebindings").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a clusterRoleBinding and creates it.  Returns the server's representation of the clusterRoleBinding, and an error, if there is any.
func (c *clusterRoleBindings) Create(clusterRoleBinding *v1.ClusterRoleBinding) (result *v1.ClusterRoleBinding, err error) {
	result = &v1.ClusterRoleBinding{}
	err = c.client.Post().
		Resource("clusterrolebindings").
		Body(clusterRoleBinding).
		Do().
		Into(result)
	return
}

// Update takes the representation of a clusterRoleBinding and updates it. Returns the server's representation of the clusterRoleBinding, and an error, if there is any.
func (c *clusterRoleBindings) Update(clusterRoleBinding *v1.ClusterRoleBinding) (result *v1.ClusterRoleBinding, err error) {
	result = &v1.ClusterRoleBinding{}
	err = c.client.Put().
		Resource("clusterrolebindings").
		Name(clusterRoleBinding.Name).
		Body(clusterRoleBinding).
		Do().
		Into(result)
	return
}

// Delete takes name of the clusterRoleBinding and deletes it. Returns an error if one occurs.
func (c *clusterRoleBindings) Delete(name string, options *meta_v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("clusterrolebindings").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterRoleBindings) DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error {
	return c.client.Delete().
		Resource("clusterrolebindings").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched clusterRoleBinding.
func (c *clusterRoleBindings) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ClusterRoleBinding, err error) {
	result = &v1.ClusterRoleBinding{}
	err = c.client.Patch(pt).
		Resource("clusterrolebindings").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

/*
Copyright 2017 The Kubernetes Authors.

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

package v1beta1

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	scheme "k8s.io/client-go/kubernetes/scheme"
	v1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	rest "k8s.io/client-go/rest"
)

// ThirdPartyResourcesGetter has a method to return a ThirdPartyResourceInterface.
// A group's client should implement this interface.
type ThirdPartyResourcesGetter interface {
	ThirdPartyResources() ThirdPartyResourceInterface
}

// ThirdPartyResourceInterface has methods to work with ThirdPartyResource resources.
type ThirdPartyResourceInterface interface {
	Create(*v1beta1.ThirdPartyResource) (*v1beta1.ThirdPartyResource, error)
	Update(*v1beta1.ThirdPartyResource) (*v1beta1.ThirdPartyResource, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1beta1.ThirdPartyResource, error)
	List(opts v1.ListOptions) (*v1beta1.ThirdPartyResourceList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.ThirdPartyResource, err error)
	ThirdPartyResourceExpansion
}

// thirdPartyResources implements ThirdPartyResourceInterface
type thirdPartyResources struct {
	client rest.Interface
}

// newThirdPartyResources returns a ThirdPartyResources
func newThirdPartyResources(c *ExtensionsV1beta1Client) *thirdPartyResources {
	return &thirdPartyResources{
		client: c.RESTClient(),
	}
}

// Create takes the representation of a thirdPartyResource and creates it.  Returns the server's representation of the thirdPartyResource, and an error, if there is any.
func (c *thirdPartyResources) Create(thirdPartyResource *v1beta1.ThirdPartyResource) (result *v1beta1.ThirdPartyResource, err error) {
	result = &v1beta1.ThirdPartyResource{}
	err = c.client.Post().
		Resource("thirdpartyresources").
		Body(thirdPartyResource).
		Do().
		Into(result)
	return
}

// Update takes the representation of a thirdPartyResource and updates it. Returns the server's representation of the thirdPartyResource, and an error, if there is any.
func (c *thirdPartyResources) Update(thirdPartyResource *v1beta1.ThirdPartyResource) (result *v1beta1.ThirdPartyResource, err error) {
	result = &v1beta1.ThirdPartyResource{}
	err = c.client.Put().
		Resource("thirdpartyresources").
		Name(thirdPartyResource.Name).
		Body(thirdPartyResource).
		Do().
		Into(result)
	return
}

// Delete takes name of the thirdPartyResource and deletes it. Returns an error if one occurs.
func (c *thirdPartyResources) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("thirdpartyresources").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *thirdPartyResources) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Resource("thirdpartyresources").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the thirdPartyResource, and returns the corresponding thirdPartyResource object, and an error if there is any.
func (c *thirdPartyResources) Get(name string, options v1.GetOptions) (result *v1beta1.ThirdPartyResource, err error) {
	result = &v1beta1.ThirdPartyResource{}
	err = c.client.Get().
		Resource("thirdpartyresources").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ThirdPartyResources that match those selectors.
func (c *thirdPartyResources) List(opts v1.ListOptions) (result *v1beta1.ThirdPartyResourceList, err error) {
	result = &v1beta1.ThirdPartyResourceList{}
	err = c.client.Get().
		Resource("thirdpartyresources").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested thirdPartyResources.
func (c *thirdPartyResources) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Resource("thirdpartyresources").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched thirdPartyResource.
func (c *thirdPartyResources) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.ThirdPartyResource, err error) {
	result = &v1beta1.ThirdPartyResource{}
	err = c.client.Patch(pt).
		Resource("thirdpartyresources").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

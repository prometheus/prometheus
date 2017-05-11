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

package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	scheme "k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/pkg/api/v1"
	rest "k8s.io/client-go/rest"
)

// ReplicationControllersGetter has a method to return a ReplicationControllerInterface.
// A group's client should implement this interface.
type ReplicationControllersGetter interface {
	ReplicationControllers(namespace string) ReplicationControllerInterface
}

// ReplicationControllerInterface has methods to work with ReplicationController resources.
type ReplicationControllerInterface interface {
	Create(*v1.ReplicationController) (*v1.ReplicationController, error)
	Update(*v1.ReplicationController) (*v1.ReplicationController, error)
	UpdateStatus(*v1.ReplicationController) (*v1.ReplicationController, error)
	Delete(name string, options *meta_v1.DeleteOptions) error
	DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error
	Get(name string, options meta_v1.GetOptions) (*v1.ReplicationController, error)
	List(opts meta_v1.ListOptions) (*v1.ReplicationControllerList, error)
	Watch(opts meta_v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ReplicationController, err error)
	ReplicationControllerExpansion
}

// replicationControllers implements ReplicationControllerInterface
type replicationControllers struct {
	client rest.Interface
	ns     string
}

// newReplicationControllers returns a ReplicationControllers
func newReplicationControllers(c *CoreV1Client, namespace string) *replicationControllers {
	return &replicationControllers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Create takes the representation of a replicationController and creates it.  Returns the server's representation of the replicationController, and an error, if there is any.
func (c *replicationControllers) Create(replicationController *v1.ReplicationController) (result *v1.ReplicationController, err error) {
	result = &v1.ReplicationController{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		Body(replicationController).
		Do().
		Into(result)
	return
}

// Update takes the representation of a replicationController and updates it. Returns the server's representation of the replicationController, and an error, if there is any.
func (c *replicationControllers) Update(replicationController *v1.ReplicationController) (result *v1.ReplicationController, err error) {
	result = &v1.ReplicationController{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		Name(replicationController.Name).
		Body(replicationController).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclientstatus=false comment above the type to avoid generating UpdateStatus().

func (c *replicationControllers) UpdateStatus(replicationController *v1.ReplicationController) (result *v1.ReplicationController, err error) {
	result = &v1.ReplicationController{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		Name(replicationController.Name).
		SubResource("status").
		Body(replicationController).
		Do().
		Into(result)
	return
}

// Delete takes name of the replicationController and deletes it. Returns an error if one occurs.
func (c *replicationControllers) Delete(name string, options *meta_v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *replicationControllers) DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the replicationController, and returns the corresponding replicationController object, and an error if there is any.
func (c *replicationControllers) Get(name string, options meta_v1.GetOptions) (result *v1.ReplicationController, err error) {
	result = &v1.ReplicationController{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ReplicationControllers that match those selectors.
func (c *replicationControllers) List(opts meta_v1.ListOptions) (result *v1.ReplicationControllerList, err error) {
	result = &v1.ReplicationControllerList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested replicationControllers.
func (c *replicationControllers) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("replicationcontrollers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched replicationController.
func (c *replicationControllers) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ReplicationController, err error) {
	result = &v1.ReplicationController{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("replicationcontrollers").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

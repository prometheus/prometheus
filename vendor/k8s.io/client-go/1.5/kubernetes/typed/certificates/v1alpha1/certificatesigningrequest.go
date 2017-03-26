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
	v1alpha1 "k8s.io/client-go/1.5/pkg/apis/certificates/v1alpha1"
	watch "k8s.io/client-go/1.5/pkg/watch"
)

// CertificateSigningRequestsGetter has a method to return a CertificateSigningRequestInterface.
// A group's client should implement this interface.
type CertificateSigningRequestsGetter interface {
	CertificateSigningRequests() CertificateSigningRequestInterface
}

// CertificateSigningRequestInterface has methods to work with CertificateSigningRequest resources.
type CertificateSigningRequestInterface interface {
	Create(*v1alpha1.CertificateSigningRequest) (*v1alpha1.CertificateSigningRequest, error)
	Update(*v1alpha1.CertificateSigningRequest) (*v1alpha1.CertificateSigningRequest, error)
	UpdateStatus(*v1alpha1.CertificateSigningRequest) (*v1alpha1.CertificateSigningRequest, error)
	Delete(name string, options *api.DeleteOptions) error
	DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error
	Get(name string) (*v1alpha1.CertificateSigningRequest, error)
	List(opts api.ListOptions) (*v1alpha1.CertificateSigningRequestList, error)
	Watch(opts api.ListOptions) (watch.Interface, error)
	Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.CertificateSigningRequest, err error)
	CertificateSigningRequestExpansion
}

// certificateSigningRequests implements CertificateSigningRequestInterface
type certificateSigningRequests struct {
	client *CertificatesClient
}

// newCertificateSigningRequests returns a CertificateSigningRequests
func newCertificateSigningRequests(c *CertificatesClient) *certificateSigningRequests {
	return &certificateSigningRequests{
		client: c,
	}
}

// Create takes the representation of a certificateSigningRequest and creates it.  Returns the server's representation of the certificateSigningRequest, and an error, if there is any.
func (c *certificateSigningRequests) Create(certificateSigningRequest *v1alpha1.CertificateSigningRequest) (result *v1alpha1.CertificateSigningRequest, err error) {
	result = &v1alpha1.CertificateSigningRequest{}
	err = c.client.Post().
		Resource("certificatesigningrequests").
		Body(certificateSigningRequest).
		Do().
		Into(result)
	return
}

// Update takes the representation of a certificateSigningRequest and updates it. Returns the server's representation of the certificateSigningRequest, and an error, if there is any.
func (c *certificateSigningRequests) Update(certificateSigningRequest *v1alpha1.CertificateSigningRequest) (result *v1alpha1.CertificateSigningRequest, err error) {
	result = &v1alpha1.CertificateSigningRequest{}
	err = c.client.Put().
		Resource("certificatesigningrequests").
		Name(certificateSigningRequest.Name).
		Body(certificateSigningRequest).
		Do().
		Into(result)
	return
}

func (c *certificateSigningRequests) UpdateStatus(certificateSigningRequest *v1alpha1.CertificateSigningRequest) (result *v1alpha1.CertificateSigningRequest, err error) {
	result = &v1alpha1.CertificateSigningRequest{}
	err = c.client.Put().
		Resource("certificatesigningrequests").
		Name(certificateSigningRequest.Name).
		SubResource("status").
		Body(certificateSigningRequest).
		Do().
		Into(result)
	return
}

// Delete takes name of the certificateSigningRequest and deletes it. Returns an error if one occurs.
func (c *certificateSigningRequests) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Resource("certificatesigningrequests").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *certificateSigningRequests) DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error {
	return c.client.Delete().
		Resource("certificatesigningrequests").
		VersionedParams(&listOptions, api.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the certificateSigningRequest, and returns the corresponding certificateSigningRequest object, and an error if there is any.
func (c *certificateSigningRequests) Get(name string) (result *v1alpha1.CertificateSigningRequest, err error) {
	result = &v1alpha1.CertificateSigningRequest{}
	err = c.client.Get().
		Resource("certificatesigningrequests").
		Name(name).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of CertificateSigningRequests that match those selectors.
func (c *certificateSigningRequests) List(opts api.ListOptions) (result *v1alpha1.CertificateSigningRequestList, err error) {
	result = &v1alpha1.CertificateSigningRequestList{}
	err = c.client.Get().
		Resource("certificatesigningrequests").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested certificateSigningRequests.
func (c *certificateSigningRequests) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Prefix("watch").
		Resource("certificatesigningrequests").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched certificateSigningRequest.
func (c *certificateSigningRequests) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.CertificateSigningRequest, err error) {
	result = &v1alpha1.CertificateSigningRequest{}
	err = c.client.Patch(pt).
		Resource("certificatesigningrequests").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}

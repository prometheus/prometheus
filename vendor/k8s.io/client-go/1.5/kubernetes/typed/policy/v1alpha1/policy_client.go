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
	registered "k8s.io/client-go/1.5/pkg/apimachinery/registered"
	serializer "k8s.io/client-go/1.5/pkg/runtime/serializer"
	rest "k8s.io/client-go/1.5/rest"
)

type PolicyInterface interface {
	GetRESTClient() *rest.RESTClient
	PodDisruptionBudgetsGetter
}

// PolicyClient is used to interact with features provided by the Policy group.
type PolicyClient struct {
	*rest.RESTClient
}

func (c *PolicyClient) PodDisruptionBudgets(namespace string) PodDisruptionBudgetInterface {
	return newPodDisruptionBudgets(c, namespace)
}

// NewForConfig creates a new PolicyClient for the given config.
func NewForConfig(c *rest.Config) (*PolicyClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &PolicyClient{client}, nil
}

// NewForConfigOrDie creates a new PolicyClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *PolicyClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new PolicyClient for the given RESTClient.
func New(c *rest.RESTClient) *PolicyClient {
	return &PolicyClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	// if policy group is not registered, return an error
	g, err := registered.Group("policy")
	if err != nil {
		return err
	}
	config.APIPath = "/apis"
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	// TODO: Unconditionally set the config.Version, until we fix the config.
	//if config.Version == "" {
	copyGroupVersion := g.GroupVersion
	config.GroupVersion = &copyGroupVersion
	//}

	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	return nil
}

// GetRESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *PolicyClient) GetRESTClient() *rest.RESTClient {
	if c == nil {
		return nil
	}
	return c.RESTClient
}

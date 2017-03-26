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

package v1

import (
	api "k8s.io/client-go/1.5/pkg/api"
	registered "k8s.io/client-go/1.5/pkg/apimachinery/registered"
	serializer "k8s.io/client-go/1.5/pkg/runtime/serializer"
	rest "k8s.io/client-go/1.5/rest"
)

type CoreInterface interface {
	GetRESTClient() *rest.RESTClient
	ComponentStatusesGetter
	ConfigMapsGetter
	EndpointsGetter
	EventsGetter
	LimitRangesGetter
	NamespacesGetter
	NodesGetter
	PersistentVolumesGetter
	PersistentVolumeClaimsGetter
	PodsGetter
	PodTemplatesGetter
	ReplicationControllersGetter
	ResourceQuotasGetter
	SecretsGetter
	ServicesGetter
	ServiceAccountsGetter
}

// CoreClient is used to interact with features provided by the Core group.
type CoreClient struct {
	*rest.RESTClient
}

func (c *CoreClient) ComponentStatuses() ComponentStatusInterface {
	return newComponentStatuses(c)
}

func (c *CoreClient) ConfigMaps(namespace string) ConfigMapInterface {
	return newConfigMaps(c, namespace)
}

func (c *CoreClient) Endpoints(namespace string) EndpointsInterface {
	return newEndpoints(c, namespace)
}

func (c *CoreClient) Events(namespace string) EventInterface {
	return newEvents(c, namespace)
}

func (c *CoreClient) LimitRanges(namespace string) LimitRangeInterface {
	return newLimitRanges(c, namespace)
}

func (c *CoreClient) Namespaces() NamespaceInterface {
	return newNamespaces(c)
}

func (c *CoreClient) Nodes() NodeInterface {
	return newNodes(c)
}

func (c *CoreClient) PersistentVolumes() PersistentVolumeInterface {
	return newPersistentVolumes(c)
}

func (c *CoreClient) PersistentVolumeClaims(namespace string) PersistentVolumeClaimInterface {
	return newPersistentVolumeClaims(c, namespace)
}

func (c *CoreClient) Pods(namespace string) PodInterface {
	return newPods(c, namespace)
}

func (c *CoreClient) PodTemplates(namespace string) PodTemplateInterface {
	return newPodTemplates(c, namespace)
}

func (c *CoreClient) ReplicationControllers(namespace string) ReplicationControllerInterface {
	return newReplicationControllers(c, namespace)
}

func (c *CoreClient) ResourceQuotas(namespace string) ResourceQuotaInterface {
	return newResourceQuotas(c, namespace)
}

func (c *CoreClient) Secrets(namespace string) SecretInterface {
	return newSecrets(c, namespace)
}

func (c *CoreClient) Services(namespace string) ServiceInterface {
	return newServices(c, namespace)
}

func (c *CoreClient) ServiceAccounts(namespace string) ServiceAccountInterface {
	return newServiceAccounts(c, namespace)
}

// NewForConfig creates a new CoreClient for the given config.
func NewForConfig(c *rest.Config) (*CoreClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &CoreClient{client}, nil
}

// NewForConfigOrDie creates a new CoreClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *CoreClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new CoreClient for the given RESTClient.
func New(c *rest.RESTClient) *CoreClient {
	return &CoreClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	// if core group is not registered, return an error
	g, err := registered.Group("")
	if err != nil {
		return err
	}
	config.APIPath = "/api"
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
func (c *CoreClient) GetRESTClient() *rest.RESTClient {
	if c == nil {
		return nil
	}
	return c.RESTClient
}

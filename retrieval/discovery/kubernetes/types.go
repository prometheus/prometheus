// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

type EventType string

const (
	added    EventType = "ADDED"
	modified EventType = "MODIFIED"
	deleted  EventType = "DELETED"
)

type nodeEvent struct {
	EventType EventType `json:"type"`
	Node      *Node     `json:"object"`
}

type serviceEvent struct {
	EventType EventType `json:"type"`
	Service   *Service  `json:"object"`
}

type endpointsEvent struct {
	EventType EventType  `json:"type"`
	Endpoints *Endpoints `json:"object"`
}

// From here down types are copied from
// https://github.com/GoogleCloudPlatform/kubernetes/blob/master/pkg/api/v1/types.go
// with all currently irrelevant types/fields stripped out. This removes the
// need for any kubernetes dependencies, with the drawback of having to keep
// this file up to date.

// ListMeta describes metadata that synthetic resources must have, including lists and
// various status objects.
type ListMeta struct {
	// An opaque value that represents the version of this response for use with optimistic
	// concurrency and change monitoring endpoints.  Clients must treat these values as opaque
	// and values may only be valid for a particular resource or set of resources. Only servers
	// will generate resource versions.
	ResourceVersion string `json:"resourceVersion,omitempty" description:"string that identifies the internal version of this object that can be used by clients to determine when objects have changed; populated by the system, read-only; value must be treated as opaque by clients and passed unmodified back to the server: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#concurrency-control-and-consistency"`
}

// ObjectMeta is metadata that all persisted resources must have, which includes all objects
// users must create.
type ObjectMeta struct {
	// Name is unique within a namespace.  Name is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	Name string `json:"name,omitempty" description:"string that identifies an object. Must be unique within a namespace; cannot be updated; see http://releases.k8s.io/HEAD/docs/user-guide/identifiers.md#names"`

	// Namespace defines the space within which name must be unique. An empty namespace is
	// equivalent to the "default" namespace, but "default" is the canonical representation.
	// Not all objects are required to be scoped to a namespace - the value of this field for
	// those objects will be empty.
	Namespace string `json:"namespace,omitempty" description:"namespace of the object; must be a DNS_LABEL; cannot be updated; see http://releases.k8s.io/HEAD/docs/user-guide/namespaces.md"`

	ResourceVersion string `json:"resourceVersion,omitempty" description:"string that identifies the internal version of this object that can be used by clients to determine when objects have changed; populated by the system, read-only; value must be treated as opaque by clients and passed unmodified back to the server: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#concurrency-control-and-consistency"`

	// TODO: replace map[string]string with labels.LabelSet type
	Labels map[string]string `json:"labels,omitempty" description:"map of string keys and values that can be used to organize and categorize objects; may match selectors of replication controllers and services; see http://releases.k8s.io/HEAD/docs/user-guide/labels.md"`

	// Annotations are unstructured key value data stored with a resource that may be set by
	// external tooling. They are not queryable and should be preserved when modifying
	// objects.
	Annotations map[string]string `json:"annotations,omitempty" description:"map of string keys and values that can be used by external tooling to store and retrieve arbitrary metadata about objects; see http://releases.k8s.io/HEAD/docs/user-guide/annotations.md"`
}

// Protocol defines network protocols supported for things like conatiner ports.
type Protocol string

const (
	// ProtocolTCP is the TCP protocol.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP is the UDP protocol.
	ProtocolUDP Protocol = "UDP"
)

const (
	// NamespaceAll is the default argument to specify on a context when you want to list or filter resources across all namespaces
	NamespaceAll string = ""
)

// Container represents a single container that is expected to be run on the host.
type Container struct {
	// Required: This must be a DNS_LABEL.  Each container in a pod must
	// have a unique name.
	Name string `json:"name" description:"name of the container; must be a DNS_LABEL and unique within the pod; cannot be updated"`
	// Optional.
	Image string `json:"image,omitempty" description:"Docker image name; see http://releases.k8s.io/HEAD/docs/user-guide/images.md"`
}

// Service is a named abstraction of software service (for example, mysql) consisting of local port
// (for example 3306) that the proxy listens on, and the selector that determines which pods
// will answer requests sent through the proxy.
type Service struct {
	ObjectMeta `json:"metadata,omitempty" description:"standard object metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	// Spec defines the behavior of a service.
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	Spec ServiceSpec `json:"spec,omitempty"`
}

// ServiceSpec describes the attributes that a user creates on a service.
type ServiceSpec struct {
	// The list of ports that are exposed by this service.
	// More info: http://releases.k8s.io/HEAD/docs/user-guide/services.md#virtual-ips-and-service-proxies
	Ports []ServicePort `json:"ports"`
}

// ServicePort conatins information on service's port.
type ServicePort struct {
	// The IP protocol for this port. Supports "TCP" and "UDP".
	// Default is TCP.
	Protocol Protocol `json:"protocol,omitempty"`

	// The port that will be exposed by this service.
	Port int32 `json:"port"`
}

// ServiceList holds a list of services.
type ServiceList struct {
	ListMeta `json:"metadata,omitempty" description:"standard list metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	Items []Service `json:"items" description:"list of services"`
}

// Endpoints is a collection of endpoints that implement the actual service.  Example:
//   Name: "mysvc",
//   Subsets: [
//     {
//       Addresses: [{"ip": "10.10.1.1"}, {"ip": "10.10.2.2"}],
//       Ports: [{"name": "a", "port": 8675}, {"name": "b", "port": 309}]
//     },
//     {
//       Addresses: [{"ip": "10.10.3.3"}],
//       Ports: [{"name": "a", "port": 93}, {"name": "b", "port": 76}]
//     },
//  ]
type Endpoints struct {
	ObjectMeta `json:"metadata,omitempty" description:"standard object metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	// The set of all endpoints is the union of all subsets.
	Subsets []EndpointSubset `json:"subsets" description:"sets of addresses and ports that comprise a service"`
}

// EndpointSubset is a group of addresses with a common set of ports.  The
// expanded set of endpoints is the Cartesian product of Addresses x Ports.
// For example, given:
//   {
//     Addresses: [{"ip": "10.10.1.1"}, {"ip": "10.10.2.2"}],
//     Ports:     [{"name": "a", "port": 8675}, {"name": "b", "port": 309}]
//   }
// The resulting set of endpoints can be viewed as:
//     a: [ 10.10.1.1:8675, 10.10.2.2:8675 ],
//     b: [ 10.10.1.1:309, 10.10.2.2:309 ]
type EndpointSubset struct {
	Addresses []EndpointAddress `json:"addresses,omitempty" description:"IP addresses which offer the related ports"`
	Ports     []EndpointPort    `json:"ports,omitempty" description:"port numbers available on the related IP addresses"`
}

// EndpointAddress is a tuple that describes single IP address.
type EndpointAddress struct {
	// The IP of this endpoint.
	// TODO: This should allow hostname or IP, see #4447.
	IP string `json:"ip" description:"IP address of the endpoint"`
}

// EndpointPort is a tuple that describes a single port.
type EndpointPort struct {
	// The port number.
	Port int `json:"port" description:"port number of the endpoint"`

	// The IP protocol for this port.
	Protocol Protocol `json:"protocol,omitempty" description:"protocol for this port; must be UDP or TCP; TCP if unspecified"`
}

// EndpointsList is a list of endpoints.
type EndpointsList struct {
	ListMeta `json:"metadata,omitempty" description:"standard list metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	Items []Endpoints `json:"items" description:"list of endpoints"`
}

// NodeStatus is information about the current status of a node.
type NodeStatus struct {
	// Queried from cloud provider, if available.
	Addresses []NodeAddress `json:"addresses,omitempty" description:"list of addresses reachable to the node; see http://releases.k8s.io/HEAD/docs/admin/node.md#node-addresses" patchStrategy:"merge" patchMergeKey:"type"`
}

type NodeAddressType string

// These are valid address type of node.
const (
	NodeHostName   NodeAddressType = "Hostname"
	NodeExternalIP NodeAddressType = "ExternalIP"
	NodeInternalIP NodeAddressType = "InternalIP"
)

type NodeAddress struct {
	Type    NodeAddressType `json:"type" description:"node address type, one of Hostname, ExternalIP or InternalIP"`
	Address string          `json:"address" description:"the node address"`
}

// Node is a worker node in Kubernetes, formerly known as minion.
// Each node will have a unique identifier in the cache (i.e. in etcd).
type Node struct {
	ObjectMeta `json:"metadata,omitempty" description:"standard object metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	// Status describes the current status of a Node
	Status NodeStatus `json:"status,omitempty" description:"most recently observed status of the node; populated by the system, read-only; http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status"`
}

// NodeList is the whole list of all Nodes which have been registered with master.
type NodeList struct {
	ListMeta `json:"metadata,omitempty" description:"standard list metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	Items []Node `json:"items" description:"list of nodes"`
}

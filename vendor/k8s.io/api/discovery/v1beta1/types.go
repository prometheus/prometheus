/*
Copyright 2019 The Kubernetes Authors.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:prerelease-lifecycle-gen:introduced=1.16
// +k8s:prerelease-lifecycle-gen:deprecated=1.22

// EndpointSlice represents a subset of the endpoints that implement a service.
// For a given service there may be multiple EndpointSlice objects, selected by
// labels, which must be joined to produce the full set of endpoints.
type EndpointSlice struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// addressType specifies the type of address carried by this EndpointSlice.
	// All addresses in this slice must be the same type. This field is
	// immutable after creation. The following address types are currently
	// supported:
	// * IPv4: Represents an IPv4 Address.
	// * IPv6: Represents an IPv6 Address.
	// * FQDN: Represents a Fully Qualified Domain Name.
	AddressType AddressType `json:"addressType" protobuf:"bytes,4,rep,name=addressType"`
	// endpoints is a list of unique endpoints in this slice. Each slice may
	// include a maximum of 1000 endpoints.
	// +listType=atomic
	Endpoints []Endpoint `json:"endpoints" protobuf:"bytes,2,rep,name=endpoints"`
	// ports specifies the list of network ports exposed by each endpoint in
	// this slice. Each port must have a unique name. When ports is empty, it
	// indicates that there are no defined ports. When a port is defined with a
	// nil port value, it indicates "all ports". Each slice may include a
	// maximum of 100 ports.
	// +optional
	// +listType=atomic
	Ports []EndpointPort `json:"ports" protobuf:"bytes,3,rep,name=ports"`
}

// AddressType represents the type of address referred to by an endpoint.
type AddressType string

const (
	// AddressTypeIP represents an IP Address.
	// This address type has been deprecated and has been replaced by the IPv4
	// and IPv6 adddress types. New resources with this address type will be
	// considered invalid. This will be fully removed in 1.18.
	// +deprecated
	AddressTypeIP = AddressType("IP")
	// AddressTypeIPv4 represents an IPv4 Address.
	AddressTypeIPv4 = AddressType(v1.IPv4Protocol)
	// AddressTypeIPv6 represents an IPv6 Address.
	AddressTypeIPv6 = AddressType(v1.IPv6Protocol)
	// AddressTypeFQDN represents a FQDN.
	AddressTypeFQDN = AddressType("FQDN")
)

// Endpoint represents a single logical "backend" implementing a service.
type Endpoint struct {
	// addresses of this endpoint. The contents of this field are interpreted
	// according to the corresponding EndpointSlice addressType field. Consumers
	// must handle different types of addresses in the context of their own
	// capabilities. This must contain at least one address but no more than
	// 100.
	// +listType=set
	Addresses []string `json:"addresses" protobuf:"bytes,1,rep,name=addresses"`
	// conditions contains information about the current status of the endpoint.
	Conditions EndpointConditions `json:"conditions,omitempty" protobuf:"bytes,2,opt,name=conditions"`
	// hostname of this endpoint. This field may be used by consumers of
	// endpoints to distinguish endpoints from each other (e.g. in DNS names).
	// Multiple endpoints which use the same hostname should be considered
	// fungible (e.g. multiple A values in DNS). Must pass DNS Label (RFC 1123)
	// validation.
	// +optional
	Hostname *string `json:"hostname,omitempty" protobuf:"bytes,3,opt,name=hostname"`
	// targetRef is a reference to a Kubernetes object that represents this
	// endpoint.
	// +optional
	TargetRef *v1.ObjectReference `json:"targetRef,omitempty" protobuf:"bytes,4,opt,name=targetRef"`
	// topology contains arbitrary topology information associated with the
	// endpoint. These key/value pairs must conform with the label format.
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// Topology may include a maximum of 16 key/value pairs. This includes, but
	// is not limited to the following well known keys:
	// * kubernetes.io/hostname: the value indicates the hostname of the node
	//   where the endpoint is located. This should match the corresponding
	//   node label.
	// * topology.kubernetes.io/zone: the value indicates the zone where the
	//   endpoint is located. This should match the corresponding node label.
	// * topology.kubernetes.io/region: the value indicates the region where the
	//   endpoint is located. This should match the corresponding node label.
	// +optional
	Topology map[string]string `json:"topology,omitempty" protobuf:"bytes,5,opt,name=topology"`
}

// EndpointConditions represents the current condition of an endpoint.
type EndpointConditions struct {
	// ready indicates that this endpoint is prepared to receive traffic,
	// according to whatever system is managing the endpoint. A nil value
	// indicates an unknown state. In most cases consumers should interpret this
	// unknown state as ready.
	// +optional
	Ready *bool `json:"ready,omitempty" protobuf:"bytes,1,name=ready"`
}

// EndpointPort represents a Port used by an EndpointSlice
type EndpointPort struct {
	// The name of this port. All ports in an EndpointSlice must have a unique
	// name. If the EndpointSlice is dervied from a Kubernetes service, this
	// corresponds to the Service.ports[].name.
	// Name must either be an empty string or pass DNS_LABEL validation:
	// * must be no more than 63 characters long.
	// * must consist of lower case alphanumeric characters or '-'.
	// * must start and end with an alphanumeric character.
	// Default is empty string.
	Name *string `json:"name,omitempty" protobuf:"bytes,1,name=name"`
	// The IP protocol for this port.
	// Must be UDP, TCP, or SCTP.
	// Default is TCP.
	Protocol *v1.Protocol `json:"protocol,omitempty" protobuf:"bytes,2,name=protocol"`
	// The port number of the endpoint.
	// If this is not specified, ports are not restricted and must be
	// interpreted in the context of the specific consumer.
	Port *int32 `json:"port,omitempty" protobuf:"bytes,3,opt,name=port"`
	// The application protocol for this port.
	// This field follows standard Kubernetes label syntax.
	// Un-prefixed names are reserved for IANA standard service names (as per
	// RFC-6335 and http://www.iana.org/assignments/service-names).
	// Non-standard protocols should use prefixed names such as
	// mycompany.com/my-custom-protocol.
	// +optional
	AppProtocol *string `json:"appProtocol,omitempty" protobuf:"bytes,4,name=appProtocol"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:prerelease-lifecycle-gen:introduced=1.16
// +k8s:prerelease-lifecycle-gen:deprecated=1.22

// EndpointSliceList represents a list of endpoint slices
type EndpointSliceList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// List of endpoint slices
	Items []EndpointSlice `json:"items" protobuf:"bytes,2,rep,name=items"`
}

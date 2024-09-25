// Copyright 2020 The Prometheus Authors
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

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// endpointSliceAdaptor is an adaptor for the different EndpointSlice versions.
type endpointSliceAdaptor interface {
	get() interface{}
	getObjectMeta() metav1.ObjectMeta
	name() string
	namespace() string
	addressType() string
	endpoints() []endpointSliceEndpointAdaptor
	ports() []endpointSlicePortAdaptor
	labels() map[string]string
	labelServiceName() string
}

type endpointSlicePortAdaptor interface {
	name() *string
	port() *int32
	protocol() *string
	appProtocol() *string
}

type endpointSliceEndpointAdaptor interface {
	addresses() []string
	hostname() *string
	nodename() *string
	zone() *string
	conditions() endpointSliceEndpointConditionsAdaptor
	targetRef() *corev1.ObjectReference
	topology() map[string]string
}

type endpointSliceEndpointConditionsAdaptor interface {
	ready() *bool
	serving() *bool
	terminating() *bool
}

// Adaptor for k8s.io/api/discovery/v1.
type endpointSliceAdaptorV1 struct {
	endpointSlice *v1.EndpointSlice
}

func newEndpointSliceAdaptorFromV1(endpointSlice *v1.EndpointSlice) endpointSliceAdaptor {
	return &endpointSliceAdaptorV1{endpointSlice: endpointSlice}
}

func (e *endpointSliceAdaptorV1) get() interface{} {
	return e.endpointSlice
}

func (e *endpointSliceAdaptorV1) getObjectMeta() metav1.ObjectMeta {
	return e.endpointSlice.ObjectMeta
}

func (e *endpointSliceAdaptorV1) name() string {
	return e.endpointSlice.ObjectMeta.Name
}

func (e *endpointSliceAdaptorV1) namespace() string {
	return e.endpointSlice.ObjectMeta.Namespace
}

func (e *endpointSliceAdaptorV1) addressType() string {
	return string(e.endpointSlice.AddressType)
}

func (e *endpointSliceAdaptorV1) endpoints() []endpointSliceEndpointAdaptor {
	eps := make([]endpointSliceEndpointAdaptor, 0, len(e.endpointSlice.Endpoints))
	for i := 0; i < len(e.endpointSlice.Endpoints); i++ {
		eps = append(eps, newEndpointSliceEndpointAdaptorFromV1(e.endpointSlice.Endpoints[i]))
	}
	return eps
}

func (e *endpointSliceAdaptorV1) ports() []endpointSlicePortAdaptor {
	ports := make([]endpointSlicePortAdaptor, 0, len(e.endpointSlice.Ports))
	for i := 0; i < len(e.endpointSlice.Ports); i++ {
		ports = append(ports, newEndpointSlicePortAdaptorFromV1(e.endpointSlice.Ports[i]))
	}
	return ports
}

func (e *endpointSliceAdaptorV1) labels() map[string]string {
	return e.endpointSlice.Labels
}

func (e *endpointSliceAdaptorV1) labelServiceName() string {
	return v1.LabelServiceName
}

type endpointSliceEndpointAdaptorV1 struct {
	endpoint v1.Endpoint
}

func newEndpointSliceEndpointAdaptorFromV1(endpoint v1.Endpoint) endpointSliceEndpointAdaptor {
	return &endpointSliceEndpointAdaptorV1{endpoint: endpoint}
}

func (e *endpointSliceEndpointAdaptorV1) addresses() []string {
	return e.endpoint.Addresses
}

func (e *endpointSliceEndpointAdaptorV1) hostname() *string {
	return e.endpoint.Hostname
}

func (e *endpointSliceEndpointAdaptorV1) nodename() *string {
	return e.endpoint.NodeName
}

func (e *endpointSliceEndpointAdaptorV1) zone() *string {
	return e.endpoint.Zone
}

func (e *endpointSliceEndpointAdaptorV1) conditions() endpointSliceEndpointConditionsAdaptor {
	return newEndpointSliceEndpointConditionsAdaptorFromV1(e.endpoint.Conditions)
}

func (e *endpointSliceEndpointAdaptorV1) targetRef() *corev1.ObjectReference {
	return e.endpoint.TargetRef
}

func (e *endpointSliceEndpointAdaptorV1) topology() map[string]string {
	return e.endpoint.DeprecatedTopology
}

type endpointSliceEndpointConditionsAdaptorV1 struct {
	endpointConditions v1.EndpointConditions
}

func newEndpointSliceEndpointConditionsAdaptorFromV1(endpointConditions v1.EndpointConditions) endpointSliceEndpointConditionsAdaptor {
	return &endpointSliceEndpointConditionsAdaptorV1{endpointConditions: endpointConditions}
}

func (e *endpointSliceEndpointConditionsAdaptorV1) ready() *bool {
	return e.endpointConditions.Ready
}

func (e *endpointSliceEndpointConditionsAdaptorV1) serving() *bool {
	return e.endpointConditions.Serving
}

func (e *endpointSliceEndpointConditionsAdaptorV1) terminating() *bool {
	return e.endpointConditions.Terminating
}

type endpointSlicePortAdaptorV1 struct {
	endpointPort v1.EndpointPort
}

func newEndpointSlicePortAdaptorFromV1(port v1.EndpointPort) endpointSlicePortAdaptor {
	return &endpointSlicePortAdaptorV1{endpointPort: port}
}

func (e *endpointSlicePortAdaptorV1) name() *string {
	return e.endpointPort.Name
}

func (e *endpointSlicePortAdaptorV1) port() *int32 {
	return e.endpointPort.Port
}

func (e *endpointSlicePortAdaptorV1) protocol() *string {
	val := string(*e.endpointPort.Protocol)
	return &val
}

func (e *endpointSlicePortAdaptorV1) appProtocol() *string {
	return e.endpointPort.AppProtocol
}

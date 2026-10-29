// Copyright The Prometheus Authors
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
	"k8s.io/client-go/kubernetes"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1client "k8s.io/client-go/kubernetes/typed/discovery/v1"
	networkingv1client "k8s.io/client-go/kubernetes/typed/networking/v1"
)

// k8sClient exposes only the API-group accessors discovery uses. It is
// satisfied by both clientAdapter and the fake clientset used in tests.
type k8sClient interface {
	CoreV1() corev1client.CoreV1Interface
	AppsV1() appsv1client.AppsV1Interface
	BatchV1() batchv1client.BatchV1Interface
	DiscoveryV1() discoveryv1client.DiscoveryV1Interface
	NetworkingV1() networkingv1client.NetworkingV1Interface
}

// clientAdapter captures the used API-group accessors as method-value closures.
// Keeping the concrete clientset out of an interface-typed field avoids forcing
// the linker, via reflection, to retain every API group and resource client,
// which reduces the binary size.
type clientAdapter struct {
	coreV1       func() corev1client.CoreV1Interface
	appsV1       func() appsv1client.AppsV1Interface
	batchV1      func() batchv1client.BatchV1Interface
	discoveryV1  func() discoveryv1client.DiscoveryV1Interface
	networkingV1 func() networkingv1client.NetworkingV1Interface
}

// newClientAdapter wraps a concrete clientset, exposing only the API groups
// discovery uses.
func newClientAdapter(c *kubernetes.Clientset) *clientAdapter {
	return &clientAdapter{
		coreV1:       c.CoreV1,
		appsV1:       c.AppsV1,
		batchV1:      c.BatchV1,
		discoveryV1:  c.DiscoveryV1,
		networkingV1: c.NetworkingV1,
	}
}

// CoreV1 returns the core/v1 API-group client.
func (a *clientAdapter) CoreV1() corev1client.CoreV1Interface { return a.coreV1() }

// AppsV1 returns the apps/v1 API-group client.
func (a *clientAdapter) AppsV1() appsv1client.AppsV1Interface { return a.appsV1() }

// BatchV1 returns the batch/v1 API-group client.
func (a *clientAdapter) BatchV1() batchv1client.BatchV1Interface { return a.batchV1() }

// DiscoveryV1 returns the discovery/v1 API-group client.
func (a *clientAdapter) DiscoveryV1() discoveryv1client.DiscoveryV1Interface {
	return a.discoveryV1()
}

// NetworkingV1 returns the networking/v1 API-group client.
func (a *clientAdapter) NetworkingV1() networkingv1client.NetworkingV1Interface {
	return a.networkingV1()
}

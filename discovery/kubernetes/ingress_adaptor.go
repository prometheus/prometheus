// Copyright 2016 The Prometheus Authors
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
	v1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
)

// ingressAdaptor is an adaptor for the different Ingress versions
type ingressAdaptor interface {
	name() string
	namespace() string
	labels() map[string]string
	annotations() map[string]string
	tlsHosts() []string
	ingressClassName() *string
	rules() []ingressRuleAdaptor
}

type ingressRuleAdaptor interface {
	paths() []string
	host() string
}

// Adaptor for networking.k8s.io/v1
type ingressAdaptorV1 struct {
	ingress *v1.Ingress
}

func newIngressAdaptorFromV1(ingress *v1.Ingress) ingressAdaptor {
	return &ingressAdaptorV1{ingress: ingress}
}

func (i *ingressAdaptorV1) name() string                   { return i.ingress.Name }
func (i *ingressAdaptorV1) namespace() string              { return i.ingress.Namespace }
func (i *ingressAdaptorV1) labels() map[string]string      { return i.ingress.Labels }
func (i *ingressAdaptorV1) annotations() map[string]string { return i.ingress.Annotations }
func (i *ingressAdaptorV1) ingressClassName() *string      { return i.ingress.Spec.IngressClassName }

func (i *ingressAdaptorV1) tlsHosts() []string {
	var hosts []string
	for _, tls := range i.ingress.Spec.TLS {
		hosts = append(hosts, tls.Hosts...)
	}
	return hosts
}

func (i *ingressAdaptorV1) rules() []ingressRuleAdaptor {
	var rules []ingressRuleAdaptor
	for _, rule := range i.ingress.Spec.Rules {
		rules = append(rules, newIngressRuleAdaptorFromV1(rule))
	}
	return rules
}

type ingressRuleAdaptorV1 struct {
	rule v1.IngressRule
}

func newIngressRuleAdaptorFromV1(rule v1.IngressRule) ingressRuleAdaptor {
	return &ingressRuleAdaptorV1{rule: rule}
}

func (i *ingressRuleAdaptorV1) paths() []string {
	rv := i.rule.IngressRuleValue
	if rv.HTTP == nil {
		return nil
	}
	paths := make([]string, len(rv.HTTP.Paths))
	for n, p := range rv.HTTP.Paths {
		paths[n] = p.Path
	}
	return paths
}

func (i *ingressRuleAdaptorV1) host() string { return i.rule.Host }

// Adaptor for networking.k8s.io/v1beta1
type ingressAdaptorV1Beta1 struct {
	ingress *v1beta1.Ingress
}

func newIngressAdaptorFromV1beta1(ingress *v1beta1.Ingress) ingressAdaptor {
	return &ingressAdaptorV1Beta1{ingress: ingress}
}

func (i *ingressAdaptorV1Beta1) name() string                   { return i.ingress.Name }
func (i *ingressAdaptorV1Beta1) namespace() string              { return i.ingress.Namespace }
func (i *ingressAdaptorV1Beta1) labels() map[string]string      { return i.ingress.Labels }
func (i *ingressAdaptorV1Beta1) annotations() map[string]string { return i.ingress.Annotations }
func (i *ingressAdaptorV1Beta1) ingressClassName() *string      { return i.ingress.Spec.IngressClassName }

func (i *ingressAdaptorV1Beta1) tlsHosts() []string {
	var hosts []string
	for _, tls := range i.ingress.Spec.TLS {
		hosts = append(hosts, tls.Hosts...)
	}
	return hosts
}

func (i *ingressAdaptorV1Beta1) rules() []ingressRuleAdaptor {
	var rules []ingressRuleAdaptor
	for _, rule := range i.ingress.Spec.Rules {
		rules = append(rules, newIngressRuleAdaptorFromV1Beta1(rule))
	}
	return rules
}

type ingressRuleAdaptorV1Beta1 struct {
	rule v1beta1.IngressRule
}

func newIngressRuleAdaptorFromV1Beta1(rule v1beta1.IngressRule) ingressRuleAdaptor {
	return &ingressRuleAdaptorV1Beta1{rule: rule}
}

func (i *ingressRuleAdaptorV1Beta1) paths() []string {
	rv := i.rule.IngressRuleValue
	if rv.HTTP == nil {
		return nil
	}
	paths := make([]string, len(rv.HTTP.Paths))
	for n, p := range rv.HTTP.Paths {
		paths[n] = p.Path
	}
	return paths
}

func (i *ingressRuleAdaptorV1Beta1) host() string { return i.rule.Host }

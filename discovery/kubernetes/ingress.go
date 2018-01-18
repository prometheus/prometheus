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
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// Ingress implements discovery of Kubernetes ingresss.
type Ingress struct {
	logger   log.Logger
	informer cache.SharedInformer
	store    cache.Store
}

// NewIngress returns a new ingress discovery.
func NewIngress(l log.Logger, inf cache.SharedInformer) *Ingress {
	return &Ingress{logger: l, informer: inf, store: inf.GetStore()}
}

// Run implements the Discoverer interface.
func (s *Ingress) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Send full initial set of pod targets.
	var initial []*targetgroup.Group
	for _, o := range s.store.List() {
		tg := s.buildIngress(o.(*v1beta1.Ingress))
		initial = append(initial, tg)
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}

	// Send target groups for ingress updates.
	send := func(tg *targetgroup.Group) {
		select {
		case <-ctx.Done():
		case ch <- []*targetgroup.Group{tg}:
		}
	}
	s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("ingress", "add").Inc()

			ingress, err := convertToIngress(o)
			if err != nil {
				level.Error(s.logger).Log("msg", "converting to Ingress object failed", "err", err.Error())
				return
			}
			send(s.buildIngress(ingress))
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("ingress", "delete").Inc()

			ingress, err := convertToIngress(o)
			if err != nil {
				level.Error(s.logger).Log("msg", "converting to Ingress object failed", "err", err.Error())
				return
			}
			send(&targetgroup.Group{Source: ingressSource(ingress)})
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("ingress", "update").Inc()

			ingress, err := convertToIngress(o)
			if err != nil {
				level.Error(s.logger).Log("msg", "converting to Ingress object failed", "err", err.Error())
				return
			}
			send(s.buildIngress(ingress))
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func convertToIngress(o interface{}) (*v1beta1.Ingress, error) {
	ingress, ok := o.(*v1beta1.Ingress)
	if ok {
		return ingress, nil
	}

	deletedState, ok := o.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, fmt.Errorf("Received unexpected object: %v", o)
	}
	ingress, ok = deletedState.Obj.(*v1beta1.Ingress)
	if !ok {
		return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Ingress object: %v", deletedState.Obj)
	}
	return ingress, nil
}

func ingressSource(s *v1beta1.Ingress) string {
	return "ingress/" + s.Namespace + "/" + s.Name
}

const (
	ingressNameLabel        = metaLabelPrefix + "ingress_name"
	ingressLabelPrefix      = metaLabelPrefix + "ingress_label_"
	ingressAnnotationPrefix = metaLabelPrefix + "ingress_annotation_"
	ingressSchemeLabel      = metaLabelPrefix + "ingress_scheme"
	ingressHostLabel        = metaLabelPrefix + "ingress_host"
	ingressPathLabel        = metaLabelPrefix + "ingress_path"
)

func ingressLabels(ingress *v1beta1.Ingress) model.LabelSet {
	ls := make(model.LabelSet, len(ingress.Labels)+len(ingress.Annotations)+2)
	ls[ingressNameLabel] = lv(ingress.Name)
	ls[namespaceLabel] = lv(ingress.Namespace)

	for k, v := range ingress.Labels {
		ln := strutil.SanitizeLabelName(ingressLabelPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}

	for k, v := range ingress.Annotations {
		ln := strutil.SanitizeLabelName(ingressAnnotationPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}
	return ls
}

func pathsFromIngressRule(rv *v1beta1.IngressRuleValue) []string {
	if rv.HTTP == nil {
		return []string{"/"}
	}
	paths := make([]string, len(rv.HTTP.Paths))
	for n, p := range rv.HTTP.Paths {
		path := p.Path
		if path == "" {
			path = "/"
		}
		paths[n] = path
	}
	return paths
}

func (s *Ingress) buildIngress(ingress *v1beta1.Ingress) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: ingressSource(ingress),
	}
	tg.Labels = ingressLabels(ingress)

	schema := "http"
	if ingress.Spec.TLS != nil {
		schema = "https"
	}
	for _, rule := range ingress.Spec.Rules {
		paths := pathsFromIngressRule(&rule.IngressRuleValue)

		for _, path := range paths {
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel: lv(rule.Host),
				ingressSchemeLabel: lv(schema),
				ingressHostLabel:   lv(rule.Host),
				ingressPathLabel:   lv(path),
			})
		}
	}

	return tg
}

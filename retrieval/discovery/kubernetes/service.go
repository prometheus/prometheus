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
	"net"
	"strconv"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
	apiv1 "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

// Service implements discovery of Kubernetes services.
type Service struct {
	logger   log.Logger
	informer cache.SharedInformer
	store    cache.Store
}

// NewService returns a new service discovery.
func NewService(l log.Logger, inf cache.SharedInformer) *Service {
	return &Service{logger: l, informer: inf, store: inf.GetStore()}
}

// Run implements the TargetProvider interface.
func (s *Service) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send full initial set of pod targets.
	var initial []*config.TargetGroup
	for _, o := range s.store.List() {
		tg := s.buildService(o.(*apiv1.Service))
		initial = append(initial, tg)
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}

	// Send target groups for service updates.
	send := func(tg *config.TargetGroup) {
		select {
		case <-ctx.Done():
		case ch <- []*config.TargetGroup{tg}:
		}
	}
	s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			send(s.buildService(o.(*apiv1.Service)))
		},
		DeleteFunc: func(o interface{}) {
			send(&config.TargetGroup{Source: serviceSource(o.(*apiv1.Service))})
		},
		UpdateFunc: func(_, o interface{}) {
			send(s.buildService(o.(*apiv1.Service)))
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func serviceSource(s *apiv1.Service) string {
	return "svc/" + s.Namespace + "/" + s.Name
}

const (
	serviceNameLabel         = metaLabelPrefix + "service_name"
	serviceLabelPrefix       = metaLabelPrefix + "service_label_"
	serviceAnnotationPrefix  = metaLabelPrefix + "service_annotation_"
	servicePortNameLabel     = metaLabelPrefix + "service_port_name"
	servicePortProtocolLabel = metaLabelPrefix + "service_port_protocol"
)

func serviceLabels(svc *apiv1.Service) model.LabelSet {
	ls := make(model.LabelSet, len(svc.Labels)+len(svc.Annotations)+2)

	ls[serviceNameLabel] = lv(svc.Name)

	for k, v := range svc.Labels {
		ln := strutil.SanitizeLabelName(serviceLabelPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}

	for k, v := range svc.Annotations {
		ln := strutil.SanitizeLabelName(serviceAnnotationPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}
	return ls
}

func (s *Service) buildService(svc *apiv1.Service) *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: serviceSource(svc),
	}
	tg.Labels = serviceLabels(svc)
	tg.Labels[namespaceLabel] = lv(svc.Namespace)

	for _, port := range svc.Spec.Ports {
		addr := net.JoinHostPort(svc.Name+"."+svc.Namespace+".svc", strconv.FormatInt(int64(port.Port), 10))

		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:       lv(addr),
			servicePortNameLabel:     lv(port.Name),
			servicePortProtocolLabel: lv(string(port.Protocol)),
		})
	}

	return tg
}

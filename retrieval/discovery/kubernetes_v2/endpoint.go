package kubernetesv2

import (
	"fmt"
	"net"
	"strconv"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/cache"
)

// Endpoints discovers new endpoint targets.
type Endpoints struct {
	logger log.Logger

	endpointsInf cache.SharedInformer
	servicesInf  cache.SharedInformer
	podsInf      cache.SharedInformer

	podStore       cache.Store
	endpointsStore cache.Store
}

// NewEndpoints returns a new endpoints discovery.
func NewEndpoints(l log.Logger, svc, eps, pod cache.SharedInformer) *Endpoints {
	ep := &Endpoints{
		logger:         l,
		endpointsInf:   eps,
		endpointsStore: eps.GetStore(),
		servicesInf:    svc,
		podsInf:        pod,
		podStore:       pod.GetStore(),
	}

	return ep
}

// Run implements the retrieval.TargetProvider interface.
func (e *Endpoints) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send full initial set of endpoint targets.
	var initial []*config.TargetGroup

	for _, o := range e.endpointsStore.List() {
		tg := e.buildEndpoints(o.(*apiv1.Endpoints))
		initial = append(initial, tg)
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}
	// Send target groups for pod updates.
	send := func(tg *config.TargetGroup) {
		if tg == nil {
			return
		}
		e.logger.With("tg", fmt.Sprintf("%#v", tg)).Debugln("endpoints update")
		select {
		case <-ctx.Done():
		case ch <- []*config.TargetGroup{tg}:
		}
	}

	e.endpointsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			send(e.buildEndpoints(o.(*apiv1.Endpoints)))
		},
		UpdateFunc: func(_, o interface{}) {
			send(e.buildEndpoints(o.(*apiv1.Endpoints)))
		},
		DeleteFunc: func(o interface{}) {
			send(&config.TargetGroup{Source: endpointsSource(o.(*apiv1.Endpoints).ObjectMeta)})
		},
	})

	serviceUpdate := func(svc *apiv1.Service) {
		ep := &apiv1.Endpoints{}
		ep.Namespace = svc.Namespace
		ep.Name = svc.Name
		obj, exists, err := e.endpointsStore.Get(ep)
		if exists && err != nil {
			send(e.buildEndpoints(obj.(*apiv1.Endpoints)))
		}
	}
	e.servicesInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(o interface{}) { serviceUpdate(o.(*apiv1.Service)) },
		UpdateFunc: func(_, o interface{}) { serviceUpdate(o.(*apiv1.Service)) },
		DeleteFunc: func(o interface{}) { serviceUpdate(o.(*apiv1.Service)) },
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func endpointsSource(ep apiv1.ObjectMeta) string {
	return "endpoints/" + ep.Namespace + "/" + ep.Name
}

const (
	serviceNameLabel        = metaLabelPrefix + "service_name"
	serviceLabelPrefix      = metaLabelPrefix + "service_label_"
	serviceAnnotationPrefix = metaLabelPrefix + "service_annotation_"

	endpointsNameLabel    = metaLabelPrefix + "endpoints_name"
	endpointReadyLabel    = metaLabelPrefix + "endpoint_ready"
	endpointPortNameLabel = metaLabelPrefix + "endpoint_port_name"
)

func (e *Endpoints) buildEndpoints(eps *apiv1.Endpoints) *config.TargetGroup {
	if len(eps.Subsets) == 0 {
		return nil
	}

	tg := &config.TargetGroup{
		Source: endpointsSource(eps.ObjectMeta),
	}
	tg.Labels = model.LabelSet{
		namespaceLabel:     lv(eps.Namespace),
		endpointsNameLabel: lv(eps.Name),
	}
	e.decorateService(eps.Namespace, eps.Name, tg)

	// type podEntry struct {
	// 	namespace string
	// 	name      string
	// }
	// seenPods := map[string]podEntry{}

	add := func(addr apiv1.EndpointAddress, port apiv1.EndpointPort, ready string) {
		a := net.JoinHostPort(addr.IP, strconv.FormatInt(int64(port.Port), 10))

		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:    lv(a),
			endpointPortNameLabel: lv(port.Name),
			endpointReadyLabel:    lv(ready),
		})
	}

	for _, ss := range eps.Subsets {
		for _, port := range ss.Ports {
			for _, addr := range ss.Addresses {
				add(addr, port, "true")
			}
			for _, addr := range ss.NotReadyAddresses {
				add(addr, port, "false")
			}
		}
	}

	return tg
}

func (e *Endpoints) resolvePodRef(ref *apiv1.ObjectReference) *apiv1.Pod {
	if ref.Kind != "Pod" {
		return nil
	}
	p, exists, err := e.podStore.Get(ref)
	if err != nil || !exists {
		return nil
	}
	return p.(*apiv1.Pod)
}

func (e *Endpoints) decorateService(ns, name string, tg *config.TargetGroup) {
	svc := &apiv1.Service{}
	svc.Namespace = ns
	svc.Name = name

	obj, exists, err := e.servicesInf.GetStore().Get(svc)
	if !exists || err != nil {
		return
	}
	svc = obj.(*apiv1.Service)

	tg.Labels[serviceNameLabel] = lv(svc.Name)
	for k, v := range svc.Labels {
		tg.Labels[serviceLabelPrefix+model.LabelName(k)] = lv(v)
	}
	for k, v := range svc.Annotations {
		tg.Labels[serviceAnnotationPrefix+model.LabelName(k)] = lv(v)
	}
}

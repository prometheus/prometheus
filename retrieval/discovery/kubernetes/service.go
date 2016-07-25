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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
)

type serviceDiscovery struct {
	mtx           sync.RWMutex
	services      map[string]map[string]*Service
	retryInterval time.Duration
	kd            *Discovery
}

func (d *serviceDiscovery) run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	update := make(chan interface{}, 10)
	go d.startServiceWatch(update, ctx.Done(), d.retryInterval)

	for {
		tgs := []*config.TargetGroup{}
		select {
		case <-ctx.Done():
			return
		case event := <-update:
			switch e := event.(type) {
			case *endpointsEvent:
				log.Debugf("k8s discovery received endpoint event (EventType=%s, Endpoint Name=%s)", e.EventType, e.Endpoints.ObjectMeta.Name)
				tgs = append(tgs, d.updateServiceEndpoints(e.Endpoints, e.EventType))
			case *serviceEvent:
				log.Debugf("k8s discovery received service event (EventType=%s, Service Name=%s)", e.EventType, e.Service.ObjectMeta.Name)
				tgs = append(tgs, d.updateService(e.Service, e.EventType))
			}
		}
		if tgs == nil {
			continue
		}

		for _, tg := range tgs {
			select {
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
				return
			}
		}
	}

}

func (d *serviceDiscovery) getServices() (map[string]map[string]*Service, string, error) {
	res, err := d.kd.queryAPIServerPath(servicesURL)
	if err != nil {
		// If we can't list services then we can't watch them. Assume this is a misconfiguration
		// & return error.
		return nil, "", fmt.Errorf("unable to list Kubernetes services: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes services; unexpected response: %d %s", res.StatusCode, res.Status)
	}
	var services ServiceList
	if err := json.NewDecoder(res.Body).Decode(&services); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes services; unexpected response body: %s", string(body))
	}

	serviceMap := map[string]map[string]*Service{}
	for idx, service := range services.Items {
		namespace, ok := serviceMap[service.ObjectMeta.Namespace]
		if !ok {
			namespace = map[string]*Service{}
			serviceMap[service.ObjectMeta.Namespace] = namespace
		}
		namespace[service.ObjectMeta.Name] = &services.Items[idx]
	}

	return serviceMap, services.ResourceVersion, nil
}

// watchServices watches services as they come & go.
func (d *serviceDiscovery) startServiceWatch(events chan<- interface{}, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		// We use separate target groups for each discovered service so we'll need to clean up any if they've been deleted
		// in Kubernetes while we couldn't connect - small chance of this, but worth dealing with.
		d.mtx.Lock()
		existingServices := d.services

		// Reset the known services.
		d.services = map[string]map[string]*Service{}
		d.mtx.Unlock()

		services, resourceVersion, err := d.getServices()
		if err != nil {
			log.Errorf("Cannot initialize services collection: %s", err)
			return
		}

		// Now let's loop through the old services & see if they still exist in here
		for oldNSName, oldNS := range existingServices {
			if ns, ok := services[oldNSName]; !ok {
				for _, service := range existingServices[oldNSName] {
					events <- &serviceEvent{Deleted, service}
				}
			} else {
				for oldServiceName, oldService := range oldNS {
					if _, ok := ns[oldServiceName]; !ok {
						events <- &serviceEvent{Deleted, oldService}
					}
				}
			}
		}

		// Discard the existing services map for GC.
		existingServices = nil

		for _, ns := range services {
			for _, service := range ns {
				events <- &serviceEvent{Added, service}
			}
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			d.watchServices(resourceVersion, events, done)
			wg.Done()
		}()
		go func() {
			d.watchServiceEndpoints(resourceVersion, events, done)
			wg.Done()
		}()

		wg.Wait()
	}, retryInterval, done)
}

func (d *serviceDiscovery) watchServices(resourceVersion string, events chan<- interface{}, done <-chan struct{}) {
	req, err := http.NewRequest("GET", servicesURL, nil)
	if err != nil {
		log.Errorf("Failed to create services request: %s", err)
		return
	}
	values := req.URL.Query()
	values.Add("watch", "true")
	values.Add("resourceVersion", resourceVersion)
	req.URL.RawQuery = values.Encode()

	res, err := d.kd.queryAPIServerReq(req)
	if err != nil {
		log.Errorf("Failed to watch services: %s", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to watch services: %d", res.StatusCode)
		return
	}

	dec := json.NewDecoder(res.Body)

	for {
		var event serviceEvent
		if err := dec.Decode(&event); err != nil {
			log.Errorf("Watch services unexpectedly closed: %s", err)
			return
		}

		select {
		case events <- &event:
		case <-done:
			return
		}
	}
}

// watchServiceEndpoints watches service endpoints as they come & go.
func (d *serviceDiscovery) watchServiceEndpoints(resourceVersion string, events chan<- interface{}, done <-chan struct{}) {
	req, err := http.NewRequest("GET", endpointsURL, nil)
	if err != nil {
		log.Errorf("Failed to create service endpoints request: %s", err)
		return
	}
	values := req.URL.Query()
	values.Add("watch", "true")
	values.Add("resourceVersion", resourceVersion)
	req.URL.RawQuery = values.Encode()

	res, err := d.kd.queryAPIServerReq(req)
	if err != nil {
		log.Errorf("Failed to watch service endpoints: %s", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to watch service endpoints: %d", res.StatusCode)
		return
	}

	dec := json.NewDecoder(res.Body)

	for {
		var event endpointsEvent
		if err := dec.Decode(&event); err != nil {
			log.Errorf("Watch service endpoints unexpectedly closed: %s", err)
			return
		}

		select {
		case events <- &event:
		case <-done:
		}
	}
}

func (d *serviceDiscovery) updateService(service *Service, eventType EventType) *config.TargetGroup {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	switch eventType {
	case Deleted:
		return d.deleteService(service)
	case Added, Modified:
		return d.addService(service)
	}
	return nil
}

func (d *serviceDiscovery) deleteService(service *Service) *config.TargetGroup {
	tg := &config.TargetGroup{Source: serviceSource(service)}

	delete(d.services[service.ObjectMeta.Namespace], service.ObjectMeta.Name)
	if len(d.services[service.ObjectMeta.Namespace]) == 0 {
		delete(d.services, service.ObjectMeta.Namespace)
	}

	return tg
}

func (d *serviceDiscovery) addService(service *Service) *config.TargetGroup {
	namespace, ok := d.services[service.ObjectMeta.Namespace]
	if !ok {
		namespace = map[string]*Service{}
		d.services[service.ObjectMeta.Namespace] = namespace
	}

	namespace[service.ObjectMeta.Name] = service
	endpointURL := fmt.Sprintf(serviceEndpointsURL, service.ObjectMeta.Namespace, service.ObjectMeta.Name)

	res, err := d.kd.queryAPIServerPath(endpointURL)
	if err != nil {
		log.Errorf("Error getting service endpoints: %s", err)
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Errorf("Failed to get service endpoints: %d", res.StatusCode)
		return nil
	}

	var eps Endpoints
	if err := json.NewDecoder(res.Body).Decode(&eps); err != nil {
		log.Errorf("Error getting service endpoints: %s", err)
		return nil
	}

	return d.updateServiceTargetGroup(service, &eps)
}

func (d *serviceDiscovery) updateServiceTargetGroup(service *Service, eps *Endpoints) *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: serviceSource(service),
		Labels: model.LabelSet{
			serviceNamespaceLabel: model.LabelValue(service.ObjectMeta.Namespace),
			serviceNameLabel:      model.LabelValue(service.ObjectMeta.Name),
		},
	}

	for k, v := range service.ObjectMeta.Labels {
		labelName := strutil.SanitizeLabelName(serviceLabelPrefix + k)
		tg.Labels[model.LabelName(labelName)] = model.LabelValue(v)
	}

	for k, v := range service.ObjectMeta.Annotations {
		labelName := strutil.SanitizeLabelName(serviceAnnotationPrefix + k)
		tg.Labels[model.LabelName(labelName)] = model.LabelValue(v)
	}

	serviceAddress := service.ObjectMeta.Name + "." + service.ObjectMeta.Namespace + ".svc"

	// Append the first TCP service port if one exists.
	for _, port := range service.Spec.Ports {
		if port.Protocol == ProtocolTCP {
			serviceAddress += fmt.Sprintf(":%d", port.Port)
			break
		}
	}
	switch d.kd.Conf.Role {
	case config.KubernetesRoleService:
		t := model.LabelSet{
			model.AddressLabel: model.LabelValue(serviceAddress),
			roleLabel:          model.LabelValue("service"),
		}
		tg.Targets = append(tg.Targets, t)

	case config.KubernetesRoleEndpoint:
		// Now let's loop through the endpoints & add them to the target group
		// with appropriate labels.
		for _, ss := range eps.Subsets {
			epPort := ss.Ports[0].Port

			for _, addr := range ss.Addresses {
				ipAddr := addr.IP
				if len(ipAddr) == net.IPv6len {
					ipAddr = "[" + ipAddr + "]"
				}
				address := fmt.Sprintf("%s:%d", ipAddr, epPort)

				t := model.LabelSet{
					model.AddressLabel: model.LabelValue(address),
					roleLabel:          model.LabelValue("endpoint"),
				}

				tg.Targets = append(tg.Targets, t)
			}
		}
	}

	return tg
}

func (d *serviceDiscovery) updateServiceEndpoints(endpoints *Endpoints, eventType EventType) *config.TargetGroup {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	serviceNamespace := endpoints.ObjectMeta.Namespace
	serviceName := endpoints.ObjectMeta.Name

	if service, ok := d.services[serviceNamespace][serviceName]; ok {
		return d.updateServiceTargetGroup(service, endpoints)
	}
	return nil
}

func serviceSource(service *Service) string {
	return sourceServicePrefix + ":" + service.ObjectMeta.Namespace + "/" + service.ObjectMeta.Name
}

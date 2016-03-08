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

package discovery

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

const (
	consulWatchTimeout  = 30 * time.Second
	consulRetryInterval = 15 * time.Second

	// consulAddressLabel is the name for the label containing a target's address.
	consulAddressLabel = model.MetaLabelPrefix + "consul_address"
	// consulNodeLabel is the name for the label containing a target's node name.
	consulNodeLabel = model.MetaLabelPrefix + "consul_node"
	// consulTagsLabel is the name of the label containing the tags assigned to the target.
	consulTagsLabel = model.MetaLabelPrefix + "consul_tags"
	// consulServiceLabel is the name of the label containing the service name.
	consulServiceLabel = model.MetaLabelPrefix + "consul_service"
	// consulServiceAddressLabel is the name of the label containing the (optional) service address.
	consulServiceAddressLabel = model.MetaLabelPrefix + "consul_service_address"
	// consulServicePortLabel is the name of the label containing the service port.
	consulServicePortLabel = model.MetaLabelPrefix + "consul_service_port"
	// consulDCLabel is the name of the label containing the datacenter ID.
	consulDCLabel = model.MetaLabelPrefix + "consul_dc"
	// consulServiceIDLabel is the name of the label containing the service ID.
	consulServiceIDLabel = model.MetaLabelPrefix + "consul_service_id"
)

// ConsulDiscovery retrieves target information from a Consul server
// and updates them via watches.
type ConsulDiscovery struct {
	client           *consul.Client
	clientConf       *consul.Config
	clientDatacenter string
	tagSeparator     string
	scrapedServices  map[string]struct{}

	mu       sync.RWMutex
	services map[string]*consulService
}

// consulService contains data belonging to the same service.
type consulService struct {
	name      string
	tgroup    config.TargetGroup
	lastIndex uint64
	removed   bool
	running   bool
	done      chan struct{}
}

// NewConsulDiscovery returns a new ConsulDiscovery for the given config.
func NewConsulDiscovery(conf *config.ConsulSDConfig) (*ConsulDiscovery, error) {
	clientConf := &consul.Config{
		Address:    conf.Server,
		Scheme:     conf.Scheme,
		Datacenter: conf.Datacenter,
		Token:      conf.Token,
		HttpAuth: &consul.HttpBasicAuth{
			Username: conf.Username,
			Password: conf.Password,
		},
	}
	client, err := consul.NewClient(clientConf)
	if err != nil {
		return nil, err
	}
	cd := &ConsulDiscovery{
		client:          client,
		clientConf:      clientConf,
		tagSeparator:    conf.TagSeparator,
		scrapedServices: map[string]struct{}{},
		services:        map[string]*consulService{},
	}
	// If the datacenter isn't set in the clientConf, let's get it from the local Consul agent
	// (Consul default is to use local node's datacenter if one isn't given for a query).
	if clientConf.Datacenter == "" {
		info, err := client.Agent().Self()
		if err != nil {
			return nil, err
		}
		cd.clientDatacenter = info["Config"]["Datacenter"].(string)
	} else {
		cd.clientDatacenter = clientConf.Datacenter
	}
	for _, name := range conf.Services {
		cd.scrapedServices[name] = struct{}{}
	}
	return cd, nil
}

// Sources implements the TargetProvider interface.
func (cd *ConsulDiscovery) Sources() []string {
	clientConf := *cd.clientConf
	clientConf.HttpClient = &http.Client{Timeout: 5 * time.Second}

	client, err := consul.NewClient(&clientConf)
	if err != nil {
		// NewClient always returns a nil error.
		panic(fmt.Errorf("discovery.ConsulDiscovery.Sources: %s", err))
	}

	srvs, _, err := client.Catalog().Services(nil)
	if err != nil {
		log.Errorf("Error refreshing service list: %s", err)
		return nil
	}
	cd.mu.Lock()
	defer cd.mu.Unlock()

	srcs := make([]string, 0, len(srvs))
	for name := range srvs {
		if _, ok := cd.scrapedServices[name]; len(cd.scrapedServices) == 0 || ok {
			srcs = append(srcs, name)
		}
	}
	return srcs
}

// Run implements the TargetProvider interface.
func (cd *ConsulDiscovery) Run(ch chan<- config.TargetGroup, done <-chan struct{}) {
	defer close(ch)
	defer cd.stop()

	update := make(chan *consulService, 10)
	go cd.watchServices(update, done)

	for {
		select {
		case <-done:
			return
		case srv := <-update:
			if srv.removed {
				close(srv.done)

				// Send clearing update.
				ch <- config.TargetGroup{Source: srv.name}
				break
			}
			// Launch watcher for the service.
			if !srv.running {
				go cd.watchService(srv, ch)
				srv.running = true
			}
		}
	}
}

func (cd *ConsulDiscovery) stop() {
	// The lock prevents Run from terminating while the watchers attempt
	// to send on their channels.
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for _, srv := range cd.services {
		close(srv.done)
	}
}

// watchServices retrieves updates from Consul's services endpoint and sends
// potential updates to the update channel.
func (cd *ConsulDiscovery) watchServices(update chan<- *consulService, done <-chan struct{}) {
	var lastIndex uint64
	for {
		catalog := cd.client.Catalog()
		srvs, meta, err := catalog.Services(&consul.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  consulWatchTimeout,
		})
		if err != nil {
			log.Errorf("Error refreshing service list: %s", err)
			time.Sleep(consulRetryInterval)
			continue
		}
		// If the index equals the previous one, the watch timed out with no update.
		if meta.LastIndex == lastIndex {
			continue
		}
		lastIndex = meta.LastIndex

		cd.mu.Lock()
		select {
		case <-done:
			cd.mu.Unlock()
			return
		default:
			// Continue.
		}
		// Check for new services.
		for name := range srvs {
			if _, ok := cd.scrapedServices[name]; len(cd.scrapedServices) > 0 && !ok {
				continue
			}
			srv, ok := cd.services[name]
			if !ok {
				srv = &consulService{
					name: name,
					done: make(chan struct{}),
				}
				srv.tgroup.Source = name
				cd.services[name] = srv
			}
			srv.tgroup.Labels = model.LabelSet{
				consulServiceLabel: model.LabelValue(name),
				consulDCLabel:      model.LabelValue(cd.clientDatacenter),
			}
			update <- srv
		}
		// Check for removed services.
		for name, srv := range cd.services {
			if _, ok := srvs[name]; !ok {
				srv.removed = true
				update <- srv
				delete(cd.services, name)
			}
		}
		cd.mu.Unlock()
	}
}

// watchService retrieves updates about srv from Consul's service endpoint.
// On a potential update the resulting target group is sent to ch.
func (cd *ConsulDiscovery) watchService(srv *consulService, ch chan<- config.TargetGroup) {
	catalog := cd.client.Catalog()
	for {
		nodes, meta, err := catalog.Service(srv.name, "", &consul.QueryOptions{
			WaitIndex: srv.lastIndex,
			WaitTime:  consulWatchTimeout,
		})
		if err != nil {
			log.Errorf("Error refreshing service %s: %s", srv.name, err)
			time.Sleep(consulRetryInterval)
			continue
		}
		// If the index equals the previous one, the watch timed out with no update.
		if meta.LastIndex == srv.lastIndex {
			continue
		}
		srv.lastIndex = meta.LastIndex
		srv.tgroup.Targets = make([]model.LabelSet, 0, len(nodes))

		for _, node := range nodes {
			addr := fmt.Sprintf("%s:%d", node.Address, node.ServicePort)
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := cd.tagSeparator + strings.Join(node.ServiceTags, cd.tagSeparator) + cd.tagSeparator

			srv.tgroup.Targets = append(srv.tgroup.Targets, model.LabelSet{
				model.AddressLabel:        model.LabelValue(addr),
				consulAddressLabel:        model.LabelValue(node.Address),
				consulNodeLabel:           model.LabelValue(node.Node),
				consulTagsLabel:           model.LabelValue(tags),
				consulServiceAddressLabel: model.LabelValue(node.ServiceAddress),
				consulServicePortLabel:    model.LabelValue(strconv.Itoa(node.ServicePort)),
				consulServiceIDLabel:      model.LabelValue(node.ServiceID),
			})
		}

		cd.mu.Lock()
		select {
		case <-srv.done:
			cd.mu.Unlock()
			return
		default:
			// Continue.
		}
		ch <- srv.tgroup
		cd.mu.Unlock()
	}
}

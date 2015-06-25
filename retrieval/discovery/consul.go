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
// limitations under the License.package discovery

package discovery

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/log"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

const (
	consulSourcePrefix  = "consul"
	consulWatchTimeout  = 30 * time.Second
	consulRetryInterval = 15 * time.Second

	// ConsuleNodeLabel is the name for the label containing a target's node name.
	ConsulNodeLabel = model.MetaLabelPrefix + "consul_node"
	// ConsulTagsLabel is the name of the label containing the tags assigned to the target.
	ConsulTagsLabel = model.MetaLabelPrefix + "consul_tags"
	// ConsulServiceLabel is the name of the label containing the service name.
	ConsulServiceLabel = model.MetaLabelPrefix + "consul_service"
	// ConsulDCLabel is the name of the label containing the datacenter ID.
	ConsulDCLabel = model.MetaLabelPrefix + "consul_dc"
)

// ConsulDiscovery retrieves target information from a Consul server
// and updates them via watches.
type ConsulDiscovery struct {
	client          *consul.Client
	clientConf      *consul.Config
	tagSeparator    string
	scrapedServices map[string]struct{}

	mu                sync.RWMutex
	services          map[string]*consulService
	runDone, srvsDone chan struct{}
}

// consulService contains data belonging to the same service.
type consulService struct {
	name      string
	tgroup    *config.TargetGroup
	lastIndex uint64
	removed   bool
	running   bool
	done      chan struct{}
}

// NewConsulDiscovery returns a new ConsulDiscovery for the given config.
func NewConsulDiscovery(conf *config.ConsulSDConfig) *ConsulDiscovery {
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
		// NewClient always returns a nil error.
		panic(fmt.Errorf("discovery.NewConsulDiscovery: %s", err))
	}
	cd := &ConsulDiscovery{
		client:          client,
		clientConf:      clientConf,
		tagSeparator:    conf.TagSeparator,
		runDone:         make(chan struct{}),
		srvsDone:        make(chan struct{}, 1),
		scrapedServices: map[string]struct{}{},
		services:        map[string]*consulService{},
	}
	for _, name := range conf.Services {
		cd.scrapedServices[name] = struct{}{}
	}
	return cd
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
		if _, ok := cd.scrapedServices[name]; ok {
			srcs = append(srcs, consulSourcePrefix+":"+name)
		}
	}
	return srcs
}

// Run implements the TargetProvider interface.
func (cd *ConsulDiscovery) Run(ch chan<- *config.TargetGroup) {
	defer close(ch)

	update := make(chan *consulService, 10)
	go cd.watchServices(update)

	for {
		select {
		case <-cd.runDone:
			return
		case srv := <-update:
			if srv.removed {
				ch <- &config.TargetGroup{Source: consulSourcePrefix + ":" + srv.name}
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

// Stop implements the TargetProvider interface.
func (cd *ConsulDiscovery) Stop() {
	log.Debugf("Stopping Consul service discovery for %s", cd.clientConf.Address)

	// The lock prevents Run from terminating while the watchers attempt
	// to send on their channels.
	cd.mu.Lock()
	defer cd.mu.Unlock()

	// The watching goroutines will terminate after their next watch timeout.
	// As this can take long, the channel is buffered and we do not wait.
	for _, srv := range cd.services {
		srv.done <- struct{}{}
	}
	cd.srvsDone <- struct{}{}

	// Terminate Run.
	cd.runDone <- struct{}{}

	log.Debugf("Consul service discovery for %s stopped.", cd.clientConf.Address)
}

// watchServices retrieves updates from Consul's services endpoint and sends
// potential updates to the update channel.
func (cd *ConsulDiscovery) watchServices(update chan<- *consulService) {
	var lastIndex uint64
	for {
		catalog := cd.client.Catalog()
		srvs, meta, err := catalog.Services(&consul.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  consulWatchTimeout,
		})
		if err != nil {
			log.Errorf("Error refreshing service list: %s", err)
			<-time.After(consulRetryInterval)
			continue
		}
		// If the index equals the previous one, the watch timed out with no update.
		if meta.LastIndex == lastIndex {
			continue
		}
		lastIndex = meta.LastIndex

		cd.mu.Lock()
		select {
		case <-cd.srvsDone:
			cd.mu.Unlock()
			return
		default:
			// Continue.
		}
		// Check for new services.
		for name := range srvs {
			if _, ok := cd.scrapedServices[name]; !ok {
				continue
			}
			srv, ok := cd.services[name]
			if !ok {
				srv = &consulService{
					name:   name,
					tgroup: &config.TargetGroup{},
					done:   make(chan struct{}, 1),
				}
				srv.tgroup.Source = consulSourcePrefix + ":" + name
				cd.services[name] = srv
			}
			srv.tgroup.Labels = model.LabelSet{
				ConsulServiceLabel: model.LabelValue(name),
				ConsulDCLabel:      model.LabelValue(cd.clientConf.Datacenter),
			}
			update <- srv
		}
		// Check for removed services.
		for name, srv := range cd.services {
			if _, ok := srvs[name]; !ok {
				srv.removed = true
				update <- srv
				srv.done <- struct{}{}
				delete(cd.services, name)
			}
		}
		cd.mu.Unlock()
	}
}

// watchService retrieves updates about srv from Consul's service endpoint.
// On a potential update the resulting target group is sent to ch.
func (cd *ConsulDiscovery) watchService(srv *consulService, ch chan<- *config.TargetGroup) {
	catalog := cd.client.Catalog()
	for {
		nodes, meta, err := catalog.Service(srv.name, "", &consul.QueryOptions{
			WaitIndex: srv.lastIndex,
			WaitTime:  consulWatchTimeout,
		})
		if err != nil {
			log.Errorf("Error refreshing service %s: %s", srv.name, err)
			<-time.After(consulRetryInterval)
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
			// Use ServiceAddress instead of Address if not empty.
			if len(node.ServiceAddress) > 0 {
				addr = fmt.Sprintf("%s:%d", node.ServiceAddress, node.ServicePort)
			}
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := cd.tagSeparator + strings.Join(node.ServiceTags, cd.tagSeparator) + cd.tagSeparator

			srv.tgroup.Targets = append(srv.tgroup.Targets, model.LabelSet{
				model.AddressLabel: model.LabelValue(addr),
				ConsulNodeLabel:    model.LabelValue(node.Node),
				ConsulTagsLabel:    model.LabelValue(tags),
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

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

package consul

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
	"golang.org/x/net/context"
)

const (
	watchTimeout  = 30 * time.Second
	retryInterval = 15 * time.Second

	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "consul_address"
	// nodeLabel is the name for the label containing a target's node name.
	nodeLabel = model.MetaLabelPrefix + "consul_node"
	// tagsLabel is the name of the label containing the tags assigned to the target.
	tagsLabel = model.MetaLabelPrefix + "consul_tags"
	// serviceLabel is the name of the label containing the service name.
	serviceLabel = model.MetaLabelPrefix + "consul_service"
	// serviceAddressLabel is the name of the label containing the (optional) service address.
	serviceAddressLabel = model.MetaLabelPrefix + "consul_service_address"
	//servicePortLabel is the name of the label containing the service port.
	servicePortLabel = model.MetaLabelPrefix + "consul_service_port"
	// datacenterLabel is the name of the label containing the datacenter ID.
	datacenterLabel = model.MetaLabelPrefix + "consul_dc"
	// serviceIDLabel is the name of the label containing the service ID.
	serviceIDLabel = model.MetaLabelPrefix + "consul_service_id"

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	rpcFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_consul_rpc_failures_total",
			Help:      "The number of Consul RPC call failures.",
		})
	rpcDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "sd_consul_rpc_duration_seconds",
			Help:      "The duration of a Consul RPC call in seconds.",
		},
		[]string{"endpoint", "call"},
	)
)

func init() {
	prometheus.MustRegister(rpcFailuresCount)
	prometheus.MustRegister(rpcDuration)

	// Initialize metric vectors.
	rpcDuration.WithLabelValues("catalog", "service")
	rpcDuration.WithLabelValues("catalog", "services")
}

// Discovery retrieves target information from a Consul server
// and updates them via watches.
type Discovery struct {
	client           *consul.Client
	clientConf       *consul.Config
	clientDatacenter string
	tagSeparator     string
	watchedServices  []string // Set of services which will be discovered.
}

// NewDiscovery returns a new Discovery for the given config.
func NewDiscovery(conf *config.ConsulSDConfig) (*Discovery, error) {
	tls, err := httputil.NewTLSConfig(conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: tls}
	wrapper := &http.Client{Transport: transport}

	clientConf := &consul.Config{
		Address:    conf.Server,
		Scheme:     conf.Scheme,
		Datacenter: conf.Datacenter,
		Token:      conf.Token,
		HttpAuth: &consul.HttpBasicAuth{
			Username: conf.Username,
			Password: conf.Password,
		},
		HttpClient: wrapper,
	}
	client, err := consul.NewClient(clientConf)
	if err != nil {
		return nil, err
	}
	cd := &Discovery{
		client:           client,
		clientConf:       clientConf,
		tagSeparator:     conf.TagSeparator,
		watchedServices:  conf.Services,
		clientDatacenter: clientConf.Datacenter,
	}
	return cd, nil
}

// shouldWatch returns whether the service of the given name should be watched.
func (d *Discovery) shouldWatch(name string) bool {
	// If there's no fixed set of watched services, we watch everything.
	if len(d.watchedServices) == 0 {
		return true
	}
	for _, sn := range d.watchedServices {
		if sn == name {
			return true
		}
	}
	return false
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Watched services and their cancelation functions.
	services := map[string]func(){}

	var lastIndex uint64
	for {
		catalog := d.client.Catalog()
		t0 := time.Now()
		srvs, meta, err := catalog.Services(&consul.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  watchTimeout,
		})
		rpcDuration.WithLabelValues("catalog", "services").Observe(time.Since(t0).Seconds())

		// We have to check the context at least once. The checks during channel sends
		// do not guarantee that.
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err != nil {
			log.Errorf("Error refreshing service list: %s", err)
			rpcFailuresCount.Inc()
			time.Sleep(retryInterval)
			continue
		}
		// If the index equals the previous one, the watch timed out with no update.
		if meta.LastIndex == lastIndex {
			continue
		}
		lastIndex = meta.LastIndex

		// If the datacenter was not set from clientConf, let's get it from the local Consul agent
		// (Consul default is to use local node's datacenter if one isn't given for a query).
		if d.clientDatacenter == "" {
			info, err := d.client.Agent().Self()
			if err != nil {
				log.Errorf("Error retrieving datacenter name: %s", err)
				time.Sleep(retryInterval)
				continue
			}
			d.clientDatacenter = info["Config"]["Datacenter"].(string)
		}

		// Check for new services.
		for name := range srvs {
			if !d.shouldWatch(name) {
				continue
			}
			if _, ok := services[name]; ok {
				continue // We are already watching the service.
			}

			srv := &consulService{
				client: d.client,
				name:   name,
				labels: model.LabelSet{
					serviceLabel:    model.LabelValue(name),
					datacenterLabel: model.LabelValue(d.clientDatacenter),
				},
				tagSeparator: d.tagSeparator,
			}

			wctx, cancel := context.WithCancel(ctx)
			go srv.watch(wctx, ch)

			services[name] = cancel
		}

		// Check for removed services.
		for name, cancel := range services {
			if _, ok := srvs[name]; !ok {
				// Call the watch cancelation function.
				cancel()
				delete(services, name)

				// Send clearing target group.
				select {
				case <-ctx.Done():
					return
				case ch <- []*config.TargetGroup{{Source: name}}:
				}
			}
		}
	}
}

// consulService contains data belonging to the same service.
type consulService struct {
	name         string
	labels       model.LabelSet
	client       *consul.Client
	tagSeparator string
}

func (srv *consulService) watch(ctx context.Context, ch chan<- []*config.TargetGroup) {
	catalog := srv.client.Catalog()

	lastIndex := uint64(0)
	for {
		t0 := time.Now()
		nodes, meta, err := catalog.Service(srv.name, "", &consul.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  watchTimeout,
		})
		rpcDuration.WithLabelValues("catalog", "service").Observe(time.Since(t0).Seconds())

		// Check the context before potentially falling in a continue-loop.
		select {
		case <-ctx.Done():
			return
		default:
			// Continue.
		}

		if err != nil {
			log.Errorf("Error refreshing service %s: %s", srv.name, err)
			rpcFailuresCount.Inc()
			time.Sleep(retryInterval)
			continue
		}
		// If the index equals the previous one, the watch timed out with no update.
		if meta.LastIndex == lastIndex {
			continue
		}
		lastIndex = meta.LastIndex

		tgroup := config.TargetGroup{
			Source:  srv.name,
			Labels:  srv.labels,
			Targets: make([]model.LabelSet, 0, len(nodes)),
		}

		for _, node := range nodes {

			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			var tags = srv.tagSeparator + strings.Join(node.ServiceTags, srv.tagSeparator) + srv.tagSeparator

			// If the service address is not empty it should be used instead of the node address
			// since the service may be registered remotely through a different node
			var addr string
			if node.ServiceAddress != "" {
				addr = net.JoinHostPort(node.ServiceAddress, fmt.Sprintf("%d", node.ServicePort))
			} else {
				addr = net.JoinHostPort(node.Address, fmt.Sprintf("%d", node.ServicePort))
			}

			tgroup.Targets = append(tgroup.Targets, model.LabelSet{
				model.AddressLabel:  model.LabelValue(addr),
				addressLabel:        model.LabelValue(node.Address),
				nodeLabel:           model.LabelValue(node.Node),
				tagsLabel:           model.LabelValue(tags),
				serviceAddressLabel: model.LabelValue(node.ServiceAddress),
				servicePortLabel:    model.LabelValue(strconv.Itoa(node.ServicePort)),
				serviceIDLabel:      model.LabelValue(node.ServiceID),
			})
		}
		// Check context twice to ensure we always catch cancelation.
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case ch <- []*config.TargetGroup{&tgroup}:
		}
	}
}

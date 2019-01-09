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
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	consul "github.com/hashicorp/consul/api"
	"github.com/mwitkow/go-conntrack"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	watchTimeout  = 30 * time.Second
	retryInterval = 15 * time.Second

	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "consul_address"
	// nodeLabel is the name for the label containing a target's node name.
	nodeLabel = model.MetaLabelPrefix + "consul_node"
	// metaDataLabel is the prefix for the labels mapping to a target's metadata.
	metaDataLabel = model.MetaLabelPrefix + "consul_metadata_"
	// serviceMetaDataLabel is the prefix for the labels mapping to a target's service metadata.
	serviceMetaDataLabel = model.MetaLabelPrefix + "consul_service_metadata_"
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
	// taggedAddressesLabel is the prefix for the labels mapping to a target's tagged addresses.
	taggedAddressesLabel = model.MetaLabelPrefix + "consul_tagged_address_"
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

	// DefaultSDConfig is the default Consul SD configuration.
	DefaultSDConfig = SDConfig{
		TagSeparator:    ",",
		Scheme:          "http",
		Server:          "localhost:8500",
		AllowStale:      true,
		RefreshInterval: model.Duration(watchTimeout),
	}
)

// SDConfig is the configuration for Consul service discovery.
type SDConfig struct {
	Server       string             `yaml:"server,omitempty"`
	Token        config_util.Secret `yaml:"token,omitempty"`
	Datacenter   string             `yaml:"datacenter,omitempty"`
	TagSeparator string             `yaml:"tag_separator,omitempty"`
	Scheme       string             `yaml:"scheme,omitempty"`
	Username     string             `yaml:"username,omitempty"`
	Password     config_util.Secret `yaml:"password,omitempty"`

	// See https://www.consul.io/docs/internals/consensus.html#consistency-modes,
	// stale reads are a lot cheaper and are a necessity if you have >5k targets.
	AllowStale bool `yaml:"allow_stale"`
	// By default use blocking queries (https://www.consul.io/api/index.html#blocking-queries)
	// but allow users to throttle updates if necessary. This can be useful because of "bugs" like
	// https://github.com/hashicorp/consul/issues/3712 which cause an un-necessary
	// amount of requests on consul.
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// See https://www.consul.io/api/catalog.html#list-services
	// The list of services for which targets are discovered.
	// Defaults to all services if empty.
	Services []string `yaml:"services,omitempty"`
	// An optional tag used to filter instances inside a service. A single tag is supported
	// here to match the Consul API.
	ServiceTag string `yaml:"tag,omitempty"`
	// Desired node metadata.
	NodeMeta map[string]string `yaml:"node_meta,omitempty"`

	TLSConfig config_util.TLSConfig `yaml:"tls_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("consul SD configuration requires a server address")
	}
	return nil
}

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
	clientDatacenter string
	tagSeparator     string
	watchedServices  []string // Set of services which will be discovered.
	watchedTag       string   // A tag used to filter instances of a service.
	watchedNodeMeta  map[string]string
	allowStale       bool
	refreshInterval  time.Duration
	finalizer        func()
	logger           log.Logger
}

// NewDiscovery returns a new Discovery for the given config.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	tls, err := config_util.NewTLSConfig(&conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		IdleConnTimeout: 5 * time.Duration(conf.RefreshInterval),
		TLSClientConfig: tls,
		DialContext: conntrack.NewDialContextFunc(
			conntrack.DialWithTracing(),
			conntrack.DialWithName("consul_sd"),
		),
	}
	wrapper := &http.Client{
		Transport: transport,
		Timeout:   35 * time.Second,
	}

	clientConf := &consul.Config{
		Address:    conf.Server,
		Scheme:     conf.Scheme,
		Datacenter: conf.Datacenter,
		Token:      string(conf.Token),
		HttpAuth: &consul.HttpBasicAuth{
			Username: conf.Username,
			Password: string(conf.Password),
		},
		HttpClient: wrapper,
	}
	client, err := consul.NewClient(clientConf)
	if err != nil {
		return nil, err
	}
	cd := &Discovery{
		client:           client,
		tagSeparator:     conf.TagSeparator,
		watchedServices:  conf.Services,
		watchedTag:       conf.ServiceTag,
		watchedNodeMeta:  conf.NodeMeta,
		allowStale:       conf.AllowStale,
		refreshInterval:  time.Duration(conf.RefreshInterval),
		clientDatacenter: conf.Datacenter,
		finalizer:        transport.CloseIdleConnections,
		logger:           logger,
	}
	return cd, nil
}

// shouldWatch returns whether the service of the given name should be watched.
func (d *Discovery) shouldWatch(name string, tags []string) bool {
	return d.shouldWatchFromName(name) && d.shouldWatchFromTags(tags)
}

// shouldWatch returns whether the service of the given name should be watched based on its name.
func (d *Discovery) shouldWatchFromName(name string) bool {
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

// shouldWatch returns whether the service of the given name should be watched based on its tags.
// This gets called when the user doesn't specify a list of services in order to avoid watching
// *all* services. Details in https://github.com/prometheus/prometheus/pull/3814
func (d *Discovery) shouldWatchFromTags(tags []string) bool {
	// If there's no fixed set of watched tags, we watch everything.
	if d.watchedTag == "" {
		return true
	}

	for _, tag := range tags {
		if d.watchedTag == tag {
			return true
		}
	}
	return false
}

// Get the local datacenter if not specified.
func (d *Discovery) getDatacenter() error {
	// If the datacenter was not set from clientConf, let's get it from the local Consul agent
	// (Consul default is to use local node's datacenter if one isn't given for a query).
	if d.clientDatacenter != "" {
		return nil
	}

	info, err := d.client.Agent().Self()
	if err != nil {
		level.Error(d.logger).Log("msg", "Error retrieving datacenter name", "err", err)
		rpcFailuresCount.Inc()
		return err
	}

	dc, ok := info["Config"]["Datacenter"].(string)
	if !ok {
		err := fmt.Errorf("invalid value '%v' for Config.Datacenter", info["Config"]["Datacenter"])
		level.Error(d.logger).Log("msg", "Error retrieving datacenter name", "err", err)
		return err
	}

	d.clientDatacenter = dc
	return nil
}

// Initialize the Discoverer run.
func (d *Discovery) initialize(ctx context.Context) {
	// Loop until we manage to get the local datacenter.
	for {
		// We have to check the context at least once. The checks during channel sends
		// do not guarantee that.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Get the local datacenter first, if necessary.
		err := d.getDatacenter()
		if err != nil {
			time.Sleep(retryInterval)
			continue
		}
		// We are good to go.
		return
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	if d.finalizer != nil {
		defer d.finalizer()
	}
	d.initialize(ctx)

	if len(d.watchedServices) == 0 || d.watchedTag != "" {
		// We need to watch the catalog.
		ticker := time.NewTicker(d.refreshInterval)

		// Watched services and their cancellation functions.
		services := make(map[string]func())
		var lastIndex uint64

		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			default:
				d.watchServices(ctx, ch, &lastIndex, services)
				<-ticker.C
			}
		}

	} else {
		// We only have fully defined services.
		for _, name := range d.watchedServices {
			d.watchService(ctx, ch, name)
		}
		<-ctx.Done()
	}
}

// Watch the catalog for new services we would like to watch. This is called only
// when we don't know yet the names of the services and need to ask Consul the
// entire list of services.
func (d *Discovery) watchServices(ctx context.Context, ch chan<- []*targetgroup.Group, lastIndex *uint64, services map[string]func()) error {
	catalog := d.client.Catalog()
	level.Debug(d.logger).Log("msg", "Watching services", "tag", d.watchedTag)

	t0 := time.Now()
	srvs, meta, err := catalog.Services(&consul.QueryOptions{
		WaitIndex:  *lastIndex,
		WaitTime:   watchTimeout,
		AllowStale: d.allowStale,
		NodeMeta:   d.watchedNodeMeta,
	})
	elapsed := time.Since(t0)
	rpcDuration.WithLabelValues("catalog", "services").Observe(elapsed.Seconds())

	if err != nil {
		level.Error(d.logger).Log("msg", "Error refreshing service list", "err", err)
		rpcFailuresCount.Inc()
		time.Sleep(retryInterval)
		return err
	}
	// If the index equals the previous one, the watch timed out with no update.
	if meta.LastIndex == *lastIndex {
		return nil
	}
	*lastIndex = meta.LastIndex

	// Check for new services.
	for name := range srvs {
		// catalog.Service() returns a map of service name to tags, we can use that to watch
		// only the services that have the tag we are looking for (if specified).
		// In the future consul will also support server side for service metadata.
		// https://github.com/hashicorp/consul/issues/1107
		if !d.shouldWatch(name, srvs[name]) {
			continue
		}
		if _, ok := services[name]; ok {
			continue // We are already watching the service.
		}

		wctx, cancel := context.WithCancel(ctx)
		d.watchService(wctx, ch, name)
		services[name] = cancel
	}

	// Check for removed services.
	for name, cancel := range services {
		if _, ok := srvs[name]; !ok {
			// Call the watch cancellation function.
			cancel()
			delete(services, name)

			// Send clearing target group.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- []*targetgroup.Group{{Source: name}}:
			}
		}
	}
	return nil
}

// consulService contains data belonging to the same service.
type consulService struct {
	name         string
	tag          string
	labels       model.LabelSet
	discovery    *Discovery
	client       *consul.Client
	tagSeparator string
	logger       log.Logger
}

// Start watching a service.
func (d *Discovery) watchService(ctx context.Context, ch chan<- []*targetgroup.Group, name string) {
	srv := &consulService{
		discovery: d,
		client:    d.client,
		name:      name,
		tag:       d.watchedTag,
		labels: model.LabelSet{
			serviceLabel:    model.LabelValue(name),
			datacenterLabel: model.LabelValue(d.clientDatacenter),
		},
		tagSeparator: d.tagSeparator,
		logger:       d.logger,
	}

	go func() {
		ticker := time.NewTicker(d.refreshInterval)
		var lastIndex uint64
		catalog := srv.client.Catalog()
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			default:
				srv.watch(ctx, ch, catalog, &lastIndex)
				<-ticker.C
			}
		}
	}()
}

// Get updates for a service.
func (srv *consulService) watch(ctx context.Context, ch chan<- []*targetgroup.Group, catalog *consul.Catalog, lastIndex *uint64) error {
	level.Debug(srv.logger).Log("msg", "Watching service", "service", srv.name, "tag", srv.tag)

	t0 := time.Now()
	nodes, meta, err := catalog.Service(srv.name, srv.tag, &consul.QueryOptions{
		WaitIndex:  *lastIndex,
		WaitTime:   watchTimeout,
		AllowStale: srv.discovery.allowStale,
		NodeMeta:   srv.discovery.watchedNodeMeta,
	})
	elapsed := time.Since(t0)
	rpcDuration.WithLabelValues("catalog", "service").Observe(elapsed.Seconds())

	// Check the context before in order to exit early.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue.
	}

	if err != nil {
		level.Error(srv.logger).Log("msg", "Error refreshing service", "service", srv.name, "tag", srv.tag, "err", err)
		rpcFailuresCount.Inc()
		time.Sleep(retryInterval)
		return err
	}
	// If the index equals the previous one, the watch timed out with no update.
	if meta.LastIndex == *lastIndex {
		return nil
	}
	*lastIndex = meta.LastIndex

	tgroup := targetgroup.Group{
		Source:  srv.name,
		Labels:  srv.labels,
		Targets: make([]model.LabelSet, 0, len(nodes)),
	}

	for _, node := range nodes {

		// We surround the separated list with the separator as well. This way regular expressions
		// in relabeling rules don't have to consider tag positions.
		var tags = srv.tagSeparator + strings.Join(node.ServiceTags, srv.tagSeparator) + srv.tagSeparator

		// If the service address is not empty it should be used instead of the node address
		// since the service may be registered remotely through a different node.
		var addr string
		if node.ServiceAddress != "" {
			addr = net.JoinHostPort(node.ServiceAddress, fmt.Sprintf("%d", node.ServicePort))
		} else {
			addr = net.JoinHostPort(node.Address, fmt.Sprintf("%d", node.ServicePort))
		}

		labels := model.LabelSet{
			model.AddressLabel:  model.LabelValue(addr),
			addressLabel:        model.LabelValue(node.Address),
			nodeLabel:           model.LabelValue(node.Node),
			tagsLabel:           model.LabelValue(tags),
			serviceAddressLabel: model.LabelValue(node.ServiceAddress),
			servicePortLabel:    model.LabelValue(strconv.Itoa(node.ServicePort)),
			serviceIDLabel:      model.LabelValue(node.ServiceID),
		}

		// Add all key/value pairs from the node's metadata as their own labels.
		for k, v := range node.NodeMeta {
			name := strutil.SanitizeLabelName(k)
			labels[metaDataLabel+model.LabelName(name)] = model.LabelValue(v)
		}

		// Add all key/value pairs from the service's metadata as their own labels.
		for k, v := range node.ServiceMeta {
			name := strutil.SanitizeLabelName(k)
			labels[serviceMetaDataLabel+model.LabelName(name)] = model.LabelValue(v)
		}

		// Add all key/value pairs from the service's tagged addresses as their own labels.
		for k, v := range node.TaggedAddresses {
			name := strutil.SanitizeLabelName(k)
			labels[taggedAddressesLabel+model.LabelName(name)] = model.LabelValue(v)
		}

		tgroup.Targets = append(tgroup.Targets, labels)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- []*targetgroup.Group{&tgroup}:
	}
	return nil
}

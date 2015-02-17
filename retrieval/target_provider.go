// Copyright 2013 The Prometheus Authors
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

package retrieval

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/golang/glog"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

const (
	sdTypeDNS  = "dns"
	sdTypeEtcd = "etcd"

	defaultResolvConf = "/etcd/resolv.conf"
)

var (
	sdLookupsCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_lookups_total",
			Help:      "The number of SD lookups.",
		},
		[]string{"type"},
	)
	sdLookupFailuresCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_lookup_failures_total",
			Help:      "The number of SD lookup failures.",
		},
		[]string{"type"},
	)
)

func init() {
	prometheus.MustRegister(sdLookupFailuresCount)
	prometheus.MustRegister(sdLookupsCount)
}

// TargetProvider encapsulates retrieving all targets for a job.
type TargetProvider interface {
	// Retrieves the current list of targets for this provider.
	Targets() ([]Target, error)
}

// sdTargetProvider provides target information based on and underlying
// TargetDiscovery from which it fetches and caches the targets.
type sdTargetProvider struct {
	job config.JobConfig

	targets []Target

	lastRefresh     time.Time
	refreshInterval time.Duration
}

// NewSDTargetProvider constructs a new sdTargetProvider for a job.
func NewSDTargetProvider(job config.JobConfig) TargetProvider {
	return &sdTargetProvider{
		job:             job,
		refreshInterval: job.ServiceDiscoveryConfig().RefreshInterval(),
	}
}

// Targets implements TargetPrvovider.
func (p *sdTargetProvider) Targets() (targets []Target, err error) {
	sd := p.job.ServiceDiscoveryConfig()
	discoveryType := sd.GetType()

	defer func() {
		sdLookupsCount.WithLabelValues(discoveryType).Inc()
		if err != nil {
			sdLookupFailuresCount.WithLabelValues(discoveryType).Inc()
		}
	}()

	if time.Since(p.lastRefresh) < p.refreshInterval {
		return p.targets, nil
	}
	baseLabels := clientmodel.LabelSet{
		clientmodel.JobLabel: clientmodel.LabelValue(p.job.GetName()),
	}
	endpoint := &url.URL{
		Scheme: "http://",
		Path:   p.job.GetMetricsPath(),
	}
	deadline := p.job.ScrapeTimeout()

	var tp TargetProvider

	switch discoveryType {
	case sdTypeDNS:
		tp = &dnsTargetProvider{sd, endpoint, baseLabels, deadline}

	case sdTypeEtcd:
		tp = &etcdTargetProvider{sd, endpoint, baseLabels, deadline}

	default:
		return nil, fmt.Errorf("unknown discovery type %s", discoveryType)
	}
	targets, err = tp.Targets()
	if err != nil {
		return nil, fmt.Errorf("SD provider: error retrieving targets via %s: %s", discoveryType, err)
	}

	p.targets = targets
	return targets, nil
}

// dnsTargetProvider provides target information from DNS.
type dnsTargetProvider struct {
	srv        config.ServiceDiscoveryConfig
	endpoint   *url.URL
	baseLabels clientmodel.LabelSet
	deadline   time.Duration
}

// Targets implements the TargetProvider interface.
func (tp *dnsTargetProvider) Targets() ([]Target, error) {
	cfgFile := tp.srv.GetConfigFile()
	if cfgFile == "" {
		cfgFile = defaultResolvConf
	}
	response, err := dnsLoopkupSRV(cfgFile, tp.srv.GetName())
	if err != nil {
		return nil, err
	}

	targets := make([]Target, 0, len(response.Answer))
	for _, record := range response.Answer {
		addr, ok := record.(*dns.SRV)
		if !ok {
			glog.Warningf("%s is not a valid SRV record", addr)
			continue
		}
		// Remove the final dot from rooted DNS names to make them look more usual.
		if addr.Target[len(addr.Target)-1] == '.' {
			addr.Target = addr.Target[:len(addr.Target)-1]
		}
		tp.endpoint.Host = fmt.Sprintf("%s:%d", addr.Target, addr.Port)
		t := NewTarget(tp.endpoint.String(), tp.deadline, tp.baseLabels)
		targets = append(targets, t)
	}
	return targets, err
}

// etcdTargetProvider provides target information from the etcd key-value store
// based on a provided config file.
type etcdTargetProvider struct {
	srv        config.ServiceDiscoveryConfig
	endpoint   *url.URL
	baseLabels clientmodel.LabelSet
	deadline   time.Duration
}

// Targets implements the TargetProvider interface.
func (tp *etcdTargetProvider) Targets() ([]Target, error) {
	cfgFile := tp.srv.GetConfigFile()
	if cfgFile == "" {
		return nil, fmt.Errorf("missing config file for etcd service discovery")
	}
	c, err := etcd.NewClientFromFile(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("etcd discovery: error creating client: %s", err)
	}
	resp, err := c.Get(tp.srv.GetName(), true, true)
	if err != nil {
		return nil, fmt.Errorf("etcd discovery: error on get request: %s", err)
	}
	// we expect the service information to be located in the node itself
	// or on the first level below.
	urls := make([]string, 0)
	if !resp.Node.Dir {
		urls = append(urls, resp.Node.Value)
	} else {
		for _, node := range resp.Node.Nodes {
			if node.Dir {
				glog.Warningf("etcd discovery: node %s is a directory, should be service entry", node.Key)
				continue
			}
			urls = append(urls, node.Value)
		}
	}

	targets := make([]Target, 0, len(urls))
	for _, u := range urls {
		// use provided endpoint as a template to fill
		// information not provided by the etcd entry.

		// missing scheme causes different behaviour for IPs and hostnames, add here if missing
		if strings.Index(u, "://") == -1 {
			u = tp.endpoint.Scheme + u
		}
		pu, err := url.Parse(u)
		if err != nil {
			glog.Warningf("etcd discovery: %s is not a valid URL: %s", u, err)
			continue
		}
		if pu.Path == "" {
			pu.Path = tp.endpoint.Path
		}
		targets = append(targets, NewTarget(pu.String(), tp.deadline, tp.baseLabels))
	}
	return targets, nil
}

func dnsLoopkupSRV(cfgFile, name string) (*dns.Msg, error) {
	conf, err := dns.ClientConfigFromFile(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("couldn't load %s: %s", cfgFile, err)
	}

	client := &dns.Client{}
	response := &dns.Msg{}

	for _, server := range conf.Servers {
		servAddr := net.JoinHostPort(server, conf.Port)
		for _, suffix := range conf.Search {
			response, err = dnsLookup(name, dns.TypeSRV, client, servAddr, suffix, false)
			if err == nil {
				if len(response.Answer) > 0 {
					return response, nil
				}
			} else {
				glog.Warningf("resolving %s.%s failed: %s", name, suffix, err)
			}
		}
		response, err = dnsLookup(name, dns.TypeSRV, client, servAddr, "", false)
		if err == nil {
			return response, err
		}
	}
	return nil, fmt.Errorf("couldn't resolve %s: No server responded", name)
}

func dnsLookup(name string, queryType uint16, client *dns.Client, servAddr string, suffix string, edns bool) (*dns.Msg, error) {
	msg := &dns.Msg{}
	lname := strings.Join([]string{name, suffix}, ".")
	msg.SetQuestion(dns.Fqdn(lname), queryType)

	if edns {
		opt := &dns.OPT{
			Hdr: dns.RR_Header{
				Name:   ".",
				Rrtype: dns.TypeOPT,
			},
		}
		opt.SetUDPSize(dns.DefaultMsgSize)
		msg.Extra = append(msg.Extra, opt)
	}

	response, _, err := client.Exchange(msg, servAddr)
	if err != nil {
		return nil, err
	}

	if msg.Id != response.Id {
		return nil, fmt.Errorf("DNS ID mismatch, request: %d, response: %d", msg.Id, response.Id)
	}
	if response.MsgHdr.Truncated {
		if client.Net == "tcp" {
			return nil, fmt.Errorf("got truncated message on tcp")
		}
		if edns { // Truncated even though EDNS is used
			client.Net = "tcp"
		}
		return dnsLookup(name, queryType, client, servAddr, suffix, !edns)
	}

	return response, nil
}

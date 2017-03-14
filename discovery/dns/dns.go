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

package dns

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

const (
	resolvConf = "/etc/resolv.conf"

	dnsNameLabel = model.MetaLabelPrefix + "dns_name"

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	dnsSDLookupsCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_dns_lookups_total",
			Help:      "The number of DNS-SD lookups.",
		})
	dnsSDLookupFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_dns_lookup_failures_total",
			Help:      "The number of DNS-SD lookup failures.",
		})
)

func init() {
	prometheus.MustRegister(dnsSDLookupFailuresCount)
	prometheus.MustRegister(dnsSDLookupsCount)
}

// Discovery periodically performs DNS-SD requests. It implements
// the TargetProvider interface.
type Discovery struct {
	names []string

	interval time.Duration
	m        sync.RWMutex
	port     int
	qtype    uint16
	lookup   bool
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *config.DNSSDConfig) *Discovery {
	qtype := dns.TypeSRV
	switch strings.ToUpper(conf.Type) {
	case "A":
		qtype = dns.TypeA
	case "AAAA":
		qtype = dns.TypeAAAA
	case "SRV":
		qtype = dns.TypeSRV
	}
	return &Discovery{
		names:    conf.Names,
		interval: time.Duration(conf.RefreshInterval),
		qtype:    qtype,
		port:     conf.Port,
		lookup:   conf.Lookup,
	}
}

// Run implements the TargetProvider interface.
func (dd *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	ticker := time.NewTicker(dd.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	dd.refreshAll(ctx, ch)

	for {
		select {
		case <-ticker.C:
			dd.refreshAll(ctx, ch)
		case <-ctx.Done():
			return
		}
	}
}

func (dd *Discovery) refreshAll(ctx context.Context, ch chan<- []*config.TargetGroup) {
	var wg sync.WaitGroup

	wg.Add(len(dd.names))
	for _, name := range dd.names {
		go func(n string) {
			if err := dd.refresh(ctx, n, ch); err != nil {
				log.Errorf("Error refreshing DNS targets: %s", err)
			}
			wg.Done()
		}(name)
	}

	wg.Wait()
}

func (dd *Discovery) refresh(ctx context.Context, name string, ch chan<- []*config.TargetGroup) error {
	response, err := lookupAll(name, dd.qtype)
	dnsSDLookupsCount.Inc()
	if err != nil {
		dnsSDLookupFailuresCount.Inc()
		return err
	}

	tg := &config.TargetGroup{}
	hostPort := func(a string, p int) model.LabelValue {
		return model.LabelValue(net.JoinHostPort(a, fmt.Sprintf("%d", p)))
	}

	for _, record := range response.Answer {
		host, port := "", 0
		switch addr := record.(type) {
		case *dns.SRV:
			// Remove the final dot from rooted DNS names to make them look more usual.
			addr.Target = strings.TrimRight(addr.Target, ".")

			host, port = addr.Target, int(addr.Port)
		case *dns.A:
			host, port = addr.A.String(), dd.port
		case *dns.AAAA:
			host, port = addr.AAAA.String(), dd.port
		default:
			log.Warnf("%q is not a valid SRV record", record)
			continue

		}

		// Try to reverse lookup for the host if it is ip addr.
		if dd.lookup {
			names, err := net.LookupAddr(host)
			if err == nil && len(names) > 0 {
				name = strings.TrimRight(names[0], ".")
			}
		}
		target := model.LabelSet{
			model.AddressLabel: hostPort(host, port),
			dnsNameLabel:       model.LabelValue(name),
		}
		tg.Targets = append(tg.Targets, target)
	}

	tg.Source = name
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- []*config.TargetGroup{tg}:
	}

	return nil
}

func lookupAll(name string, qtype uint16) (*dns.Msg, error) {
	conf, err := dns.ClientConfigFromFile(resolvConf)
	if err != nil {
		return nil, fmt.Errorf("could not load resolv.conf: %s", err)
	}

	client := &dns.Client{}
	response := &dns.Msg{}

	for _, server := range conf.Servers {
		servAddr := net.JoinHostPort(server, conf.Port)
		for _, lname := range conf.NameList(name) {
			response, err = lookup(lname, qtype, client, servAddr, false)
			if err != nil {
				log.
					With("server", server).
					With("name", name).
					With("reason", err).
					Warn("DNS resolution failed.")
				continue
			}
			if len(response.Answer) > 0 {
				return response, nil
			}
		}
	}
	return response, fmt.Errorf("could not resolve %s: no server responded", name)
}

func lookup(lname string, queryType uint16, client *dns.Client, servAddr string, edns bool) (*dns.Msg, error) {
	msg := &dns.Msg{}
	msg.SetQuestion(dns.Fqdn(lname), queryType)

	if edns {
		msg.SetEdns0(dns.DefaultMsgSize, false)
	}

	response, _, err := client.Exchange(msg, servAddr)
	if err == dns.ErrTruncated {
		if client.Net == "tcp" {
			return nil, fmt.Errorf("got truncated message on TCP (64kiB limit exceeded?)")
		}
		if edns { // Truncated even though EDNS is used
			client.Net = "tcp"
		}
		return lookup(lname, queryType, client, servAddr, !edns)
	}
	if err != nil {
		return nil, err
	}
	if msg.Id != response.Id {
		return nil, fmt.Errorf("DNS ID mismatch, request: %d, response: %d", msg.Id, response.Id)
	}
	return response, nil
}

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

package mdns

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/hashicorp/mdns"
	"github.com/prometheus/prometheus/config"
)

const (
	resolvConf = "/etc/resolv.conf"

	dnsNameLabel = model.MetaLabelPrefix + "mdns_name"

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	mdnsSDLookupsCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "mdns_sd_lookups_total",
			Help:      "The number of mDNS-SD lookups.",
		})
	mdnsSDLookupFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "mdns_sd_lookup_failures_total",
			Help:      "The number of mDNS-SD lookup failures.",
		})
)

func init() {
	prometheus.MustRegister(mdnsSDLookupFailuresCount)
	prometheus.MustRegister(mdnsSDLookupsCount)
}

// Discovery periodically performs DNS-SD requests. It implements
// the TargetProvider interface.
type Discovery struct {
	names []string

	interval time.Duration
	m        sync.RWMutex
	port     int
	qtype    uint16
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *config.MDNSConfig) *Discovery {
	return &Discovery{
		interval: time.Duration(10 * time.Second),
	}
}

// Run implements the TargetProvider interface.
func (dd *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

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

	names := []string{
		"_prometheus-http._tcp",
		"_prometheus-https._tcp",
	}

	wg.Add(len(names))
	for _, name := range names {
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
	fmt.Println("mdns.refresh", ctx, name)

	// Set up output channel and read discovered data
	responses := make(chan *mdns.ServiceEntry, 100)
	tg := &config.TargetGroup{}
	go func() {
		for response := range responses {
			labelSet := model.LabelSet{
				//dnsNameLabel:       model.LabelValue(name),
				dnsNameLabel:        model.LabelValue(response.Host),
				model.InstanceLabel: model.LabelValue(strings.TrimRight(response.Host, ".")),
				model.SchemeLabel:   model.LabelValue("http"),
			}

			// Set model.SchemeLabel to 'http' or 'https'
			if strings.Contains(response.Name, "_prometheus-https._tcp") {
				labelSet[model.SchemeLabel] = model.LabelValue("https")
			}

			// Figure out an address
			addr := model.LabelValue(fmt.Sprintf("%s:%d", response.Host, response.Port))

			if response.AddrV4 != nil {
				addr = model.LabelValue(fmt.Sprintf("%s:%d", response.AddrV4, response.Port))
			} else if response.AddrV6 != nil {
				addr = model.LabelValue(fmt.Sprintf("[%s]:%d", response.AddrV6, response.Port))
			}
			labelSet[model.AddressLabel] = addr

			// TODO: if HasTXT, parse InfoFields and set path as
			// model.MetricsPathLabel if it's there.

			tg.Targets = append(tg.Targets, labelSet)
		}
		tg.Source = name
	}()

	// Do the actual lookup
	err := mdns.Lookup(name, responses)
	close(responses)
	mdnsSDLookupsCount.Inc()

	// Fail...
	if err != nil {
		mdnsSDLookupFailuresCount.Inc()
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- []*config.TargetGroup{tg}:
	}

	return nil
}

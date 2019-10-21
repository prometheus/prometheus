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
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
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

	// DefaultSDConfig is the default DNS SD configuration.
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(30 * time.Second),
		Type:            "SRV",
	}
)

// SDConfig is the configuration for DNS based service discovery.
type SDConfig struct {
	Names           []string       `yaml:"names"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Type            string         `yaml:"type"`
	Port            int            `yaml:"port"` // Ignored for SRV records
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Names) == 0 {
		return errors.New("DNS-SD config must contain at least one SRV record name")
	}
	switch strings.ToUpper(c.Type) {
	case "SRV":
	case "A", "AAAA":
		if c.Port == 0 {
			return errors.New("a port is required in DNS-SD configs for all record types except SRV")
		}
	default:
		return errors.Errorf("invalid DNS-SD records type %s", c.Type)
	}
	return nil
}

func init() {
	prometheus.MustRegister(dnsSDLookupFailuresCount)
	prometheus.MustRegister(dnsSDLookupsCount)
}

// Discovery periodically performs DNS-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	names  []string
	port   int
	qtype  uint16
	logger log.Logger

	lookupFn func(name string, qtype uint16, logger log.Logger) (*dns.Msg, error)
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	qtype := dns.TypeSRV
	switch strings.ToUpper(conf.Type) {
	case "A":
		qtype = dns.TypeA
	case "AAAA":
		qtype = dns.TypeAAAA
	case "SRV":
		qtype = dns.TypeSRV
	}
	d := &Discovery{
		names:    conf.Names,
		qtype:    qtype,
		port:     conf.Port,
		logger:   logger,
		lookupFn: lookupWithSearchPath,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"dns",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	var (
		wg  sync.WaitGroup
		ch  = make(chan *targetgroup.Group)
		tgs = make([]*targetgroup.Group, 0, len(d.names))
	)

	wg.Add(len(d.names))
	for _, name := range d.names {
		go func(n string) {
			if err := d.refreshOne(ctx, n, ch); err != nil && err != context.Canceled {
				level.Error(d.logger).Log("msg", "Error refreshing DNS targets", "err", err)
			}
			wg.Done()
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for tg := range ch {
		tgs = append(tgs, tg)
	}
	return tgs, nil
}

func (d *Discovery) refreshOne(ctx context.Context, name string, ch chan<- *targetgroup.Group) error {
	response, err := d.lookupFn(name, d.qtype, d.logger)
	dnsSDLookupsCount.Inc()
	if err != nil {
		dnsSDLookupFailuresCount.Inc()
		return err
	}

	tg := &targetgroup.Group{}
	hostPort := func(a string, p int) model.LabelValue {
		return model.LabelValue(net.JoinHostPort(a, fmt.Sprintf("%d", p)))
	}

	for _, record := range response.Answer {
		var target model.LabelValue
		switch addr := record.(type) {
		case *dns.SRV:
			// Remove the final dot from rooted DNS names to make them look more usual.
			addr.Target = strings.TrimRight(addr.Target, ".")

			target = hostPort(addr.Target, int(addr.Port))
		case *dns.A:
			target = hostPort(addr.A.String(), d.port)
		case *dns.AAAA:
			target = hostPort(addr.AAAA.String(), d.port)
		default:
			level.Warn(d.logger).Log("msg", "Invalid SRV record", "record", record)
			continue
		}
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel: target,
			dnsNameLabel:       model.LabelValue(name),
		})
	}

	tg.Source = name
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- tg:
	}

	return nil
}

// lookupWithSearchPath tries to get an answer for various permutations of
// the given name, appending the system-configured search path as necessary.
//
// There are three possible outcomes:
//
// 1. One of the permutations of the given name is recognized as
//    "valid" by the DNS, in which case we consider ourselves "done"
//    and that answer is returned.  Note that, due to the way the DNS
//    handles "name has resource records, but none of the specified type",
//    the answer received may have an empty set of results.
//
// 2.  All of the permutations of the given name are responded to by one of
//    the servers in the "nameservers" list with the answer "that name does
//    not exist" (NXDOMAIN).  In that case, it can be considered
//    pseudo-authoritative that there are no records for that name.
//
// 3.  One or more of the names was responded to by all servers with some
//    sort of error indication.  In that case, we can't know if, in fact,
//    there are records for the name or not, so whatever state the
//    configuration is in, we should keep it that way until we know for
//    sure (by, presumably, all the names getting answers in the future).
//
// Outcomes 1 and 2 are indicated by a valid response message (possibly an
// empty one) and no error.  Outcome 3 is indicated by an error return.  The
// error will be generic-looking, because trying to return all the errors
// returned by the combination of all name permutations and servers is a
// nightmare.
func lookupWithSearchPath(name string, qtype uint16, logger log.Logger) (*dns.Msg, error) {
	conf, err := dns.ClientConfigFromFile(resolvConf)
	if err != nil {
		return nil, errors.Wrap(err, "could not load resolv.conf")
	}

	allResponsesValid := true

	for _, lname := range conf.NameList(name) {
		response, err := lookupFromAnyServer(lname, qtype, conf, logger)

		if err != nil {
			// We can't go home yet, because a later name
			// may give us a valid, successful answer.  However
			// we can no longer say "this name definitely doesn't
			// exist", because we did not get that answer for
			// at least one name.
			allResponsesValid = false
		} else if response.Rcode == dns.RcodeSuccess {
			// Outcome 1: GOLD!
			return response, nil
		}
	}

	if allResponsesValid {
		// Outcome 2: everyone says NXDOMAIN, that's good enough for me
		return &dns.Msg{}, nil
	}
	// Outcome 3: boned.
	return nil, errors.Errorf("could not resolve %q: all servers responded with errors to at least one search domain", name)
}

// lookupFromAnyServer uses all configured servers to try and resolve a specific
// name.  If a viable answer is received from a server, then it is
// immediately returned, otherwise the other servers in the config are
// tried, and if none of them return a viable answer, an error is returned.
//
// A "viable answer" is one which indicates either:
//
// 1. "yes, I know that name, and here are its records of the requested type"
//    (RCODE==SUCCESS, ANCOUNT > 0);
// 2. "yes, I know that name, but it has no records of the requested type"
//    (RCODE==SUCCESS, ANCOUNT==0); or
// 3. "I know that name doesn't exist" (RCODE==NXDOMAIN).
//
// A non-viable answer is "anything else", which encompasses both various
// system-level problems (like network timeouts) and also
// valid-but-unexpected DNS responses (SERVFAIL, REFUSED, etc).
func lookupFromAnyServer(name string, qtype uint16, conf *dns.ClientConfig, logger log.Logger) (*dns.Msg, error) {
	client := &dns.Client{}

	for _, server := range conf.Servers {
		servAddr := net.JoinHostPort(server, conf.Port)
		msg, err := askServerForName(name, qtype, client, servAddr, true)
		if err != nil {
			level.Warn(logger).Log("msg", "DNS resolution failed", "server", server, "name", name, "err", err)
			continue
		}

		if msg.Rcode == dns.RcodeSuccess || msg.Rcode == dns.RcodeNameError {
			// We have our answer.  Time to go home.
			return msg, nil
		}
	}

	return nil, errors.Errorf("could not resolve %s: no servers returned a viable answer", name)
}

// askServerForName makes a request to a specific DNS server for a specific
// name (and qtype).  Retries with TCP in the event of response truncation,
// but otherwise just sends back whatever the server gave, whether that be a
// valid-looking response, or an error.
func askServerForName(name string, queryType uint16, client *dns.Client, servAddr string, edns bool) (*dns.Msg, error) {
	msg := &dns.Msg{}

	msg.SetQuestion(dns.Fqdn(name), queryType)
	if edns {
		msg.SetEdns0(dns.DefaultMsgSize, false)
	}

	response, _, err := client.Exchange(msg, servAddr)
	if err != nil {
		return nil, err
	}

	if response.Truncated {
		if client.Net == "tcp" {
			return nil, errors.New("got truncated message on TCP (64kiB limit exceeded?)")
		}

		client.Net = "tcp"
		return askServerForName(name, queryType, client, servAddr, false)
	}

	return response, nil
}

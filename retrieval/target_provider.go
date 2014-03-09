// Copyright 2013 Prometheus Team
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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/miekg/dns"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/utility"
)

const resolvConf = "/etc/resolv.conf"

// TargetProvider encapsulates retrieving all targets for a job.
type TargetProvider interface {
	// Retrieves the current list of targets for this provider.
	Targets() ([]Target, error)
}

type sdTargetProvider struct {
	job config.JobConfig

	targets []Target

	lastRefresh     time.Time
	refreshInterval time.Duration
}

// Constructs a new sdTargetProvider for a job.
func NewSdTargetProvider(job config.JobConfig) *sdTargetProvider {
	i, err := utility.StringToDuration(job.GetSdRefreshInterval())
	if err != nil {
		panic(fmt.Sprintf("illegal refresh duration string %s: %s", job.GetSdRefreshInterval(), err))
	}
	return &sdTargetProvider{
		job:             job,
		refreshInterval: i,
	}
}

func (p *sdTargetProvider) Targets() ([]Target, error) {
	var err error
	defer func() { recordOutcome(err) }()

	if time.Since(p.lastRefresh) < p.refreshInterval {
		return p.targets, nil
	}

	response, err := lookupSRV(p.job.GetSdName())

	if err != nil {
		return nil, err
	}

	baseLabels := clientmodel.LabelSet{
		clientmodel.JobLabel: clientmodel.LabelValue(p.job.GetName()),
	}

	targets := make([]Target, 0, len(response.Answer))
	endpoint := &url.URL{
		Scheme: "http",
		Path:   p.job.GetMetricsPath(),
	}
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
		endpoint.Host = fmt.Sprintf("%s:%d", addr.Target, addr.Port)
		t := NewTarget(endpoint.String(), p.job.ScrapeTimeout(), baseLabels)
		targets = append(targets, t)
	}

	p.targets = targets
	return targets, nil
}

func lookupSRV(name string) (*dns.Msg, error) {
	conf, err := dns.ClientConfigFromFile(resolvConf)
	if err != nil {
		return nil, fmt.Errorf("Couldn't load resolv.conf: %s", err)
	}

	port, err := strconv.Atoi(conf.Port)
	if err != nil {
		return nil, fmt.Errorf("Invalid dns port in %s", resolvConf)
	}

	client := &dns.Client{}
	response := &dns.Msg{}

	for _, server := range conf.Servers {
		servAddr := addrFromServer(server)
		for _, suffix := range conf.Search {
			response, err = lookup(name, dns.TypeSRV, client, servAddr, port, suffix, false)
			if err == nil {
				if len(response.Answer) > 0 {
					return response, nil
				}
			} else {
				glog.Warningf("Resolving %s.%s failed: %s", name, suffix, err)
			}
		}
		response, err = lookup(name, dns.TypeSRV, client, servAddr, port, "", false)
		if err == nil {
			return response, nil
		}
	}
	return response, fmt.Errorf("Couldn't resolve %s: No server responded", name)
}

func addrFromServer(s string) string {
	if strings.ContainsRune(s, ':') {
		return fmt.Sprintf("[%s]:%d")
	} else {
		return fmt.Sprintf("%s:%d")
	}
}

func lookup(name string, queryType uint16, client *dns.Client, servAddr string, port int, suffix string, edns bool) (*dns.Msg, error) {
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
			return nil, fmt.Errorf("Got truncated message on tcp")
		}

		if edns { // Truncated even though EDNS is used
			client.Net = "tcp"
		}

		return lookup(name, queryType, client, servAddr, port, suffix, !edns)
	}

	return response, nil
}

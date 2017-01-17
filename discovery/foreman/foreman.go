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

package foreman

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	// metaLabelPrefix is the meta prefix used for all meta labels in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "foreman_"

	// addressLabel is the name for the label containing a target's address.
	addressLabel = metaLabelPrefix + "address"
	// serviceAddressLabel is the name for the label containing a target's service address.
	serviceAddressLabel = metaLabelPrefix + "service_address"
	// servicePortLabel is the name for the label containing a target's service port.
	servicePortLabel = metaLabelPrefix + "service_port"
	// environment is the name for the label containing a Foreman environment
	environmentLabel model.LabelName = metaLabelPrefix + "environment"
	// hostgroup is the name for a label containing a Foreman hostgroup
	hostgroupLabel model.LabelName = metaLabelPrefix + "hostgroup"

	// Constants for instrumentation.
	namespace = "prometheus"
)

var (
	refreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sd_foreman_refresh_failures_total",
			Help:      "The number of Foreman-SD refresh failures.",
		})
	refreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "sd_foreman_refresh_duration_seconds",
			Help:      "The duration of a Foreman-SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(refreshFailuresCount)
	prometheus.MustRegister(refreshDuration)
}

// there is no way to disable pagination so pick an arbitrarily large value
const hostsEndpoint string = "/api/v2/hosts/?per_page=1000000"

// Discovery provides service discovery based on a Foreman server.
type Discovery struct {
	client          *http.Client
	server          string
	username        string
	password        string
	queries         []string
	port            int
	refreshInterval time.Duration
	cachedHosts     map[string]*config.TargetGroup
}

// Initialize sets up the discovery for usage.
func NewDiscovery(conf *config.ForemanSDConfig) (*Discovery, error) {
	tls, err := httputil.NewTLSConfig(conf.TLSConfig)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: time.Duration(conf.Timeout),
		Transport: &http.Transport{
			TLSClientConfig: tls,
		},
	}

	return &Discovery{
		client:          client,
		server:          conf.Server,
		username:        conf.Username,
		password:        conf.Password,
		queries:         conf.Queries,
		port:            conf.Port,
		refreshInterval: time.Duration(conf.RefreshInterval),
	}, nil
}

// Run implements the TargetProvider interface.
func (fd *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// do an initial load of the foreman host services
	err := fd.refreshHosts(ctx, ch)
	if err != nil {
		log.Errorf("Error while refreshing hosts: %s", err)
	}
	// refresh foreman host services on refresh interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(fd.refreshInterval):
			err := fd.refreshHosts(ctx, ch)
			if err != nil {
				log.Errorf("Error while refreshing hosts: %s", err)
			}
		}
	}
}

func (fd *Discovery) refreshHosts(ctx context.Context, ch chan<- []*config.TargetGroup) (err error) {
	t0 := time.Now()
	defer func() {
		refreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			refreshFailuresCount.Inc()
		}
	}()

	targetMap, err := fd.fetchTargetGroups()
	if err != nil {
		return err
	}

	all := make([]*config.TargetGroup, 0, len(targetMap))
	for _, tg := range targetMap {
		all = append(all, tg)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- all:
	}

	// Remove hosts which no longer exist
	for source := range fd.cachedHosts {
		_, ok := targetMap[source]
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- []*config.TargetGroup{{Source: source}}:
				log.Debugf("Removing group for %s", source)
			}
		}
	}

	fd.cachedHosts = targetMap
	return nil
}

// fetchTargetGroups retrieves hosts and converts them to a map of target groups
func (fd *Discovery) fetchTargetGroups() (map[string]*config.TargetGroup, error) {
	var (
		groups = map[string]*config.TargetGroup{}
		hosts  = []*Host{}
	)
	if len(fd.queries) > 0 {
		for _, query := range fd.queries {
			hostList, err := fd.fetchHosts(fmt.Sprintf("%s%s&search=%s", fd.server, hostsEndpoint, url.QueryEscape(query)))
			if err != nil {
				return nil, err
			}
			hosts = append(hosts, hostList.Hosts...)
		}
	} else {
		// fetch all hosts since no queries were specified
		hostList, err := fd.fetchHosts(fmt.Sprintf("%s%s", fd.server, hostsEndpoint))
		if err != nil {
			return nil, err
		}
		hosts = hostList.Hosts
	}
	for _, host := range hosts {
		group := createTargetGroup(host, fd.port)
		groups[group.Source] = group
	}
	return groups, nil
}

// Host is a single host entry from a Foreman query
type Host struct {
	Hostgroup   string `json:"hostgroup_name"`
	Environment string `json:"environment_name"`
	Hostname    string `json:"name"`
}

// HostList is a list of hosts returned by a Foreman query
type HostList struct {
	Hosts []*Host `json:"results"`
}

// fetchHosts queries a Foreman server for hosts
func (fd *Discovery) fetchHosts(url string) (*HostList, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(fd.username, fd.password)
	resp, err := fd.client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	hosts := &HostList{}
	if err := json.Unmarshal(body, hosts); err != nil {
		return nil, err
	}

	return hosts, nil
}

func createTargetGroup(host *Host, port int) *config.TargetGroup {
	target := net.JoinHostPort(host.Hostname, fmt.Sprintf("%d", port))
	group := &config.TargetGroup{
		Targets: []model.LabelSet{
			model.LabelSet{
				model.AddressLabel:  model.LabelValue(target),
				addressLabel:        model.LabelValue(host.Hostname),
				serviceAddressLabel: model.LabelValue(host.Hostname),
				servicePortLabel:    model.LabelValue(strconv.Itoa(port)),
			},
		},
		Labels: model.LabelSet{
			environmentLabel: model.LabelValue(host.Environment),
			hostgroupLabel:   model.LabelValue(host.Hostgroup),
		},
		Source: target,
	}

	return group
}

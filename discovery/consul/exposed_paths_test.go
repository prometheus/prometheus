// Copyright The Prometheus Authors
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
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

// These tests use the public YAML and health-response interfaces so they can
// also exercise the pristine implementation without requiring new symbols.
func TestExposedPathRoles(t *testing.T) {
	for _, role := range []string{"", "service", "service_exposed_paths"} {
		t.Run(role, func(t *testing.T) {
			var conf SDConfig
			require.NoError(t, yaml.Unmarshal([]byte("role: '"+role+"'"), &conf))
			encoded, err := yaml.Marshal(conf)
			require.NoError(t, err)
			expected := role
			if expected == "" {
				expected = "service"
			}
			require.Contains(t, string(encoded), "role: "+expected+"\n")
		})
	}
	var conf SDConfig
	require.ErrorContains(t, yaml.Unmarshal([]byte("role: invalid"), &conf), "unknown consul SD role")
	conf = DefaultSDConfig
	role := reflect.ValueOf(&conf).Elem().FieldByName("Role")
	require.True(t, role.IsValid(), "Role must be validated for programmatic callers")
	role.SetString("invalid")
	metrics := NewTestMetrics(t, &conf, prometheus.NewRegistry())
	_, err := NewDiscovery(&conf, nil, metrics)
	require.ErrorContains(t, err, "unknown consul SD role")
}

func TestExposedPathTargets(t *testing.T) {
	// Exercise the actual configuration reference example, so role and relabel
	// changes cannot silently leave readers with a stale scrape configuration.
	doc, err := os.ReadFile("../../docs/configuration/configuration.md")
	require.NoError(t, err)
	_, section, found := strings.Cut(string(doc), "### `<consul_sd_config>`")
	require.True(t, found)
	section, _, found = strings.Cut(section, "### `<digitalocean_sd_config>`")
	require.True(t, found)
	var example struct {
		ScrapeConfigs []struct {
			JobName         string            `yaml:"job_name"`
			ConsulSDConfigs []*SDConfig       `yaml:"consul_sd_configs"`
			RelabelConfigs  []*relabel.Config `yaml:"relabel_configs"`
		} `yaml:"scrape_configs"`
	}
	for _, block := range strings.Split(section, "```yaml\n")[1:] {
		snippet, _, _ := strings.Cut(block, "```")
		if strings.Contains(snippet, "job_name: consul-exposed-metrics") {
			require.NoError(t, yaml.UnmarshalStrict([]byte(snippet), &example))
		}
	}
	require.Len(t, example.ScrapeConfigs, 1)
	job := example.ScrapeConfigs[0]
	require.Equal(t, "consul-exposed-metrics", job.JobName)
	require.Len(t, job.ConsulSDConfigs, 1)
	require.Equal(t, []string{"app-sidecar-proxy"}, job.ConsulSDConfigs[0].Services)
	require.NotEmpty(t, job.RelabelConfigs)
	for _, rule := range job.RelabelConfigs {
		require.NoError(t, rule.Validate(model.UTF8Validation))
	}
	for _, tc := range []struct{ name, serviceAddress, nodeAddress string }{
		{"service-address", "proxy-21000.example", "192.0.2.1"},
		{"node-fallback", "", "192.0.2.2"},
		{"service-ipv6", "2001:db8::21000", "192.0.2.3"},
		{"node-ipv6", "", "2001:db8::2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := []consul.ExposePath{
				{Path: "/metrics", ListenerPort: 21500, LocalPathPort: 9100},
				{Path: "/debug", ListenerPort: 21500, LocalPathPort: 9200},
				{Path: "/metrics", ListenerPort: 21500, LocalPathPort: 9200, Protocol: "http2"},
				{Path: "/custom", ListenerPort: 21600, LocalPathPort: 9300, Protocol: "custom"},
			}
			proxy := &consul.AgentServiceConnectProxyConfig{
				DestinationServiceName: "app", DestinationServiceID: "app-1",
				LocalServiceAddress: "127.0.0.1", LocalServicePort: 8080,
				Expose: consul.ExposeConfig{Paths: paths},
			}
			entry := &consul.ServiceEntry{
				Node:    &consul.Node{Node: "node", Address: tc.nodeAddress, Meta: map[string]string{"rack.name": "r1"}, TaggedAddresses: map[string]string{"lan": "192.0.2.9"}},
				Service: &consul.AgentService{Kind: consul.ServiceKindConnectProxy, ID: "proxy-1", Service: "app-sidecar-proxy", Address: tc.serviceAddress, Port: 21000, Namespace: "ns", Partition: "part", Tags: []string{"a", "b"}, Meta: map[string]string{"owner.name": "team"}, Proxy: proxy},
				Checks:  consul.HealthChecks{&consul.HealthCheck{Status: consul.HealthPassing}},
			}
			// Ordinary services and gateways must not become path targets even if they
			// happen to carry proxy data. Nil proxies and check-only exposure are safe.
			entries := []*consul.ServiceEntry{entry}
			for _, kind := range []consul.ServiceKind{"", consul.ServiceKindTerminatingGateway} {
				other := *entry
				service := *entry.Service
				service.Kind = kind
				other.Service = &service
				entries = append(entries, &other)
			}
			for _, p := range []*consul.AgentServiceConnectProxyConfig{nil, {Expose: consul.ExposeConfig{Checks: true}}} {
				other := *entry
				service := *entry.Service
				service.Proxy = p
				other.Service = &service
				entries = append(entries, &other)
			}
			response := entries
			index := 0
			stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/health/service/app-sidecar-proxy", r.URL.Path)
				require.Equal(t, "tag", r.URL.Query().Get("tag"))
				require.Equal(t, "Service.Meta.enabled == true", r.URL.Query().Get("filter"))
				index++
				w.Header().Set("X-Consul-Index", strconv.Itoa(index))
				require.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer stub.Close()
			conf := *job.ConsulSDConfigs[0]
			conf.Server = strings.TrimPrefix(stub.URL, "http://")
			conf.HealthFilter = "Service.Meta.enabled == true"
			d := newDiscovery(t, &conf)
			defer d.finalizer()
			srv := &consulService{name: "app-sidecar-proxy", tags: []string{"tag"}, labels: model.LabelSet{serviceLabel: "app-sidecar-proxy", datacenterLabel: "dc"}, discovery: d, client: d.client, tagSeparator: ",", logger: d.logger, rpcFailuresCount: d.metrics.rpcFailuresCount, serviceRPCDuration: d.metrics.serviceRPCDuration}
			ch := make(chan []*targetgroup.Group, 1)
			var lastIndex uint64
			srv.watch(context.Background(), ch, d.client.Health(), &lastIndex)
			groups := <-ch
			require.Len(t, groups, 1)
			require.Equal(t, "app-sidecar-proxy", groups[0].Source)
			require.Equal(t, srv.labels, groups[0].Labels)
			require.Len(t, groups[0].Targets, len(paths), "only explicit connect-proxy paths produce targets")
			host := tc.serviceAddress
			if host == "" {
				host = tc.nodeAddress
			}
			base := model.LabelSet{
				model.AddressLabel:      model.LabelValue(net.JoinHostPort(host, "21500")),
				"__meta_consul_address": model.LabelValue(tc.nodeAddress), "__meta_consul_node": "node",
				"__meta_consul_namespace": "ns", "__meta_consul_partition": "part", "__meta_consul_tags": ",a,b,",
				"__meta_consul_service_address": model.LabelValue(tc.serviceAddress), "__meta_consul_service_port": "21000", "__meta_consul_service_id": "proxy-1", "__meta_consul_health": "passing",
				"__meta_consul_metadata_rack_name": "r1", "__meta_consul_service_metadata_owner_name": "team", "__meta_consul_tagged_address_lan": "192.0.2.9",
				"__meta_consul_service_kind": "connect-proxy", "__meta_consul_proxy_service": "app", "__meta_consul_proxy_service_id": "app-1", "__meta_consul_proxy_service_address": "127.0.0.1", "__meta_consul_proxy_service_port": "8080",
			}
			for i, path := range paths {
				expected := base.Clone()
				expected[model.AddressLabel] = model.LabelValue(net.JoinHostPort(host, strconv.Itoa(path.ListenerPort)))
				expected["__meta_consul_proxy_exposed_path"] = model.LabelValue(path.Path)
				expected["__meta_consul_proxy_exposed_path_port"] = model.LabelValue(strconv.Itoa(path.ListenerPort))
				expected["__meta_consul_proxy_exposed_path_local_port"] = model.LabelValue(strconv.Itoa(path.LocalPathPort))
				protocol := path.Protocol
				if protocol == "" {
					protocol = "http"
				}
				expected["__meta_consul_proxy_exposed_path_protocol"] = model.LabelValue(protocol)
				require.Equal(t, expected, groups[0].Targets[i])
			}
			var scraped []labels.Labels
			for _, target := range groups[0].Targets {
				values := make(map[string]string, len(target))
				for name, value := range target {
					values[string(name)] = string(value)
				}
				builder := labels.NewBuilder(labels.FromMap(values))
				if relabel.ProcessBuilder(builder, job.RelabelConfigs...) {
					scraped = append(scraped, builder.Labels())
				}
			}
			require.Len(t, scraped, 1, "the documented job keeps only the HTTP metrics path")
			require.Equal(t, net.JoinHostPort(host, "21500"), scraped[0].Get(string(model.AddressLabel)))
			require.Equal(t, "/metrics", scraped[0].Get(string(model.MetricsPathLabel)))
			require.Equal(t, "http", scraped[0].Get("__meta_consul_proxy_exposed_path_protocol"))
			require.Empty(t, scraped[0].Get(string(model.SchemeLabel)), "Consul protocol must not become a scrape URL scheme")
			groups[0].Targets[0]["mutation"] = "first-only"
			require.NotContains(t, groups[0].Targets[1], model.LabelName("mutation"))
			response = nil
			srv.watch(context.Background(), ch, d.client.Health(), &lastIndex)
			require.Empty(t, (<-ch)[0].Targets, "an empty update clears previously exposed paths")

			// The default role keeps one target per API entry, including gateways and
			// proxies, without injecting path metadata or an empty kind label.
			response = entries
			defaultConf := DefaultSDConfig
			defaultConf.Server = conf.Server
			defaultConf.HealthFilter = conf.HealthFilter
			if field := reflect.ValueOf(&defaultConf).Elem().FieldByName("Role"); field.IsValid() {
				field.SetString("")
			}
			defaultDiscovery := newDiscovery(t, &defaultConf)
			defer defaultDiscovery.finalizer()
			srv.discovery = defaultDiscovery
			srv.watch(context.Background(), ch, defaultDiscovery.client.Health(), &lastIndex)
			defaults := (<-ch)[0].Targets
			require.Len(t, defaults, len(entries))
			for _, target := range defaults {
				require.Equal(t, model.LabelValue(net.JoinHostPort(host, "21000")), target[model.AddressLabel])
				require.NotContains(t, target, model.LabelName("__meta_consul_proxy_exposed_path"))
			}
			require.NotContains(t, defaults[1], model.LabelName("__meta_consul_service_kind"))
			require.NotContains(t, defaults[1], model.LabelName("__meta_consul_proxy_service"))
			require.Equal(t, model.LabelValue("app"), defaults[0]["__meta_consul_proxy_service"])
		})
	}
}

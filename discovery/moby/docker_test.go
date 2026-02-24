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

package moby

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
)

func TestDockerSDRefresh(t *testing.T) {
	sdmock := NewSDMock(t, "dockerprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
host: %s
`, url)
	var cfg DockerSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "docker_swarm",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 8)

	expected := []model.LabelSet{
		{
			"__address__":                "172.19.0.2:9100",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "node",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "node",
			"__meta_docker_container_name":                             "/dockersd_node_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.2",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9100",
		},
		{
			"__address__":                "172.19.0.3:80",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "noport",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "noport",
			"__meta_docker_container_name":                             "/dockersd_noport_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.3",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
		},
		{
			"__address__":                "localhost",
			"__meta_docker_container_id": "54ed6cc5c0988260436cb0e739b7b6c9cad6c439a93b4c4fdbe9753e1c94b189",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "host_networking",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_name":                             "/dockersd_host_networking_1",
			"__meta_docker_container_network_mode":                     "host",
			"__meta_docker_network_ip":                                 "",
		},
		{
			"__address__":                "172.20.0.2:3306",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.20.0.2:33060",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                "172.20.0.2:9104",
			"__meta_docker_container_id": "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                "172.20.0.3:3306",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.20.0.3:33060",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
	}
	sortFunc(expected)
	sortFunc(tg.Targets)

	for i, lbls := range expected {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func TestDockerSDRefreshMatchAllNetworks(t *testing.T) {
	sdmock := NewSDMock(t, "dockerprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
host: %s
`, url)
	var cfg DockerSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	cfg.MatchFirstNetwork = false
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()
	d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "docker_swarm",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 13)

	expected := []model.LabelSet{
		{
			"__address__":                "172.19.0.2:9100",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "node",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "node",
			"__meta_docker_container_name":                             "/dockersd_node_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.2",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9100",
		},
		{
			"__address__":                "172.19.0.3:80",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "noport",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "noport",
			"__meta_docker_container_name":                             "/dockersd_noport_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.3",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
		},
		{
			"__address__":                "localhost",
			"__meta_docker_container_id": "54ed6cc5c0988260436cb0e739b7b6c9cad6c439a93b4c4fdbe9753e1c94b189",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "host_networking",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_name":                             "/dockersd_host_networking_1",
			"__meta_docker_container_network_mode":                     "host",
			"__meta_docker_network_ip":                                 "",
		},
		{
			"__address__":                "172.20.0.2:3306",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.20.0.2:33060",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                "172.21.0.2:3306",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.21.0.2:33060",
			"__meta_docker_container_id": "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                "172.21.0.2:9104",
			"__meta_docker_container_id": "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                "172.20.0.2:9104",
			"__meta_docker_container_id": "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                "172.20.0.3:3306",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.20.0.3:33060",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                "172.21.0.3:3306",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.3",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                "172.21.0.3:33060",
			"__meta_docker_container_id": "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.3",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
	}

	sortFunc(expected)
	sortFunc(tg.Targets)

	for i, lbls := range expected {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func sortFunc(labelSets []model.LabelSet) {
	sort.Slice(labelSets, func(i, j int) bool {
		return labelSets[i]["__address__"] < labelSets[j]["__address__"]
	})
}
